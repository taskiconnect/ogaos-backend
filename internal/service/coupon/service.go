package coupon

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	apperr "ogaos-backend/internal/pkg/errors"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func normalizePlans(plans []string) pq.StringArray {
	out := make([]string, 0, len(plans))
	seen := make(map[string]struct{})

	for _, p := range plans {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	return pq.StringArray(out)
}

func fromDB(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.Wrap(apperr.CodeNotFound, "coupon not found", err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "coupons_code_key":
				return apperr.Wrap(apperr.CodeConflict, "a coupon with this code already exists", err)
			default:
				return apperr.Wrap(apperr.CodeConflict, "resource already exists", err)
			}
		case "23503":
			return apperr.Wrap(apperr.CodeBadRequest, "one or more selected records are invalid", err)
		case "23514":
			return apperr.Wrap(apperr.CodeBadRequest, "one or more submitted values are invalid", err)
		case "22P02":
			return apperr.Wrap(apperr.CodeBadRequest, "invalid input format", err)
		default:
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
	}

	return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
}

func (s *Service) Create(c *models.Coupon) error {
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	c.Description = strings.TrimSpace(c.Description)
	c.DiscountType = strings.ToLower(strings.TrimSpace(c.DiscountType))

	if c.Code == "" {
		return apperr.New(apperr.CodeBadRequest, "coupon code is required")
	}
	if c.DiscountType == "" {
		c.DiscountType = "percentage"
	}
	if c.DiscountType != "percentage" {
		return apperr.New(apperr.CodeBadRequest, "only percentage coupons are currently supported")
	}
	if c.DiscountValue < 1 || c.DiscountValue > 100 {
		return apperr.New(apperr.CodeBadRequest, "discount value must be between 1 and 100")
	}
	if c.StartsAt.IsZero() || c.ExpiresAt.IsZero() {
		return apperr.New(apperr.CodeBadRequest, "start and expiry dates are required")
	}
	if !c.ExpiresAt.After(c.StartsAt) {
		return apperr.New(apperr.CodeBadRequest, "expiry date must be after start date")
	}

	plans := normalizePlans([]string(c.ApplicablePlans))
	if len(plans) == 0 {
		return apperr.New(apperr.CodeBadRequest, "at least one applicable plan is required")
	}
	c.ApplicablePlans = plans

	if err := s.db.Create(c).Error; err != nil {
		return fromDB(err)
	}

	return nil
}

func (s *Service) List() ([]map[string]interface{}, error) {
	var coupons []models.Coupon
	if err := s.db.Where("deleted_at IS NULL").Order("created_at DESC").Find(&coupons).Error; err != nil {
		return nil, fromDB(err)
	}

	result := make([]map[string]interface{}, 0, len(coupons))
	for _, c := range coupons {
		var count int64
		if err := s.db.Model(&models.CouponRedemption{}).
			Where("coupon_id = ?", c.ID).
			Count(&count).Error; err != nil {
			return nil, fromDB(err)
		}

		remaining := "unlimited"
		if c.MaxRedemptions > 0 {
			left := c.MaxRedemptions - int(count)
			if left < 0 {
				left = 0
			}
			remaining = ""
			result = append(result, map[string]interface{}{
				"id":               c.ID,
				"code":             c.Code,
				"description":      c.Description,
				"discount_type":    c.DiscountType,
				"discount_value":   c.DiscountValue,
				"applicable_plans": c.ApplicablePlans,
				"starts_at":        c.StartsAt,
				"expires_at":       c.ExpiresAt,
				"max_redemptions":  c.MaxRedemptions,
				"redemptions":      count,
				"remaining":        left,
				"is_active":        c.IsActive,
				"is_used":          c.IsUsed,
				"created_at":       c.CreatedAt,
				"updated_at":       c.UpdatedAt,
			})
			continue
		}

		result = append(result, map[string]interface{}{
			"id":               c.ID,
			"code":             c.Code,
			"description":      c.Description,
			"discount_type":    c.DiscountType,
			"discount_value":   c.DiscountValue,
			"applicable_plans": c.ApplicablePlans,
			"starts_at":        c.StartsAt,
			"expires_at":       c.ExpiresAt,
			"max_redemptions":  c.MaxRedemptions,
			"redemptions":      count,
			"remaining":        remaining,
			"is_active":        c.IsActive,
			"is_used":          c.IsUsed,
			"created_at":       c.CreatedAt,
			"updated_at":       c.UpdatedAt,
		})
	}

	return result, nil
}

func (s *Service) Get(id uuid.UUID) (*models.Coupon, error) {
	var c models.Coupon
	if err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&c).Error; err != nil {
		return nil, fromDB(err)
	}
	return &c, nil
}

func (s *Service) Update(id uuid.UUID, updates map[string]interface{}) error {
	safeUpdates := map[string]interface{}{}

	if v, ok := updates["code"]; ok {
		code, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "code must be a string")
		}
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			return apperr.New(apperr.CodeBadRequest, "coupon code is required")
		}
		safeUpdates["code"] = code
	}

	if v, ok := updates["description"]; ok {
		desc, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "description must be a string")
		}
		safeUpdates["description"] = strings.TrimSpace(desc)
	}

	if v, ok := updates["discount_type"]; ok {
		discountType, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "discount type must be a string")
		}
		discountType = strings.ToLower(strings.TrimSpace(discountType))
		if discountType != "percentage" {
			return apperr.New(apperr.CodeBadRequest, "only percentage coupons are currently supported")
		}
		safeUpdates["discount_type"] = discountType
	}

	if v, ok := updates["discount_value"]; ok {
		switch n := v.(type) {
		case float64:
			if int(n) < 1 || int(n) > 100 {
				return apperr.New(apperr.CodeBadRequest, "discount value must be between 1 and 100")
			}
			safeUpdates["discount_value"] = int(n)
		case int:
			if n < 1 || n > 100 {
				return apperr.New(apperr.CodeBadRequest, "discount value must be between 1 and 100")
			}
			safeUpdates["discount_value"] = n
		default:
			return apperr.New(apperr.CodeBadRequest, "discount value must be a number")
		}
	}

	if v, ok := updates["max_redemptions"]; ok {
		switch n := v.(type) {
		case float64:
			if int(n) < 0 {
				return apperr.New(apperr.CodeBadRequest, "max redemptions cannot be negative")
			}
			safeUpdates["max_redemptions"] = int(n)
		case int:
			if n < 0 {
				return apperr.New(apperr.CodeBadRequest, "max redemptions cannot be negative")
			}
			safeUpdates["max_redemptions"] = n
		default:
			return apperr.New(apperr.CodeBadRequest, "max redemptions must be a number")
		}
	}

	if v, ok := updates["applicable_plans"]; ok {
		switch raw := v.(type) {
		case []string:
			plans := normalizePlans(raw)
			if len(plans) == 0 {
				return apperr.New(apperr.CodeBadRequest, "at least one applicable plan is required")
			}
			safeUpdates["applicable_plans"] = plans
		case []interface{}:
			plans := make([]string, 0, len(raw))
			for _, item := range raw {
				s, ok := item.(string)
				if !ok {
					return apperr.New(apperr.CodeBadRequest, "applicable plans must be a list of strings")
				}
				plans = append(plans, s)
			}
			normalized := normalizePlans(plans)
			if len(normalized) == 0 {
				return apperr.New(apperr.CodeBadRequest, "at least one applicable plan is required")
			}
			safeUpdates["applicable_plans"] = normalized
		default:
			return apperr.New(apperr.CodeBadRequest, "applicable plans must be a list of strings")
		}
	}

	if v, ok := updates["starts_at"]; ok {
		s, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "starts_at must be a valid datetime string")
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return apperr.Wrap(apperr.CodeBadRequest, "starts_at must be a valid RFC3339 datetime", err)
		}
		safeUpdates["starts_at"] = t
	}

	if v, ok := updates["expires_at"]; ok {
		s, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "expires_at must be a valid datetime string")
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return apperr.Wrap(apperr.CodeBadRequest, "expires_at must be a valid RFC3339 datetime", err)
		}
		safeUpdates["expires_at"] = t
	}

	if v, ok := updates["is_active"]; ok {
		b, ok := v.(bool)
		if !ok {
			return apperr.New(apperr.CodeBadRequest, "is_active must be a boolean")
		}
		safeUpdates["is_active"] = b
	}

	if len(safeUpdates) == 0 {
		return apperr.New(apperr.CodeBadRequest, "no valid update fields were provided")
	}

	if startsAt, ok := safeUpdates["starts_at"].(time.Time); ok {
		if expiresAt, ok2 := safeUpdates["expires_at"].(time.Time); ok2 && !expiresAt.After(startsAt) {
			return apperr.New(apperr.CodeBadRequest, "expiry date must be after start date")
		}
	}

	var existing models.Coupon
	if err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&existing).Error; err != nil {
		return fromDB(err)
	}

	finalStartsAt := existing.StartsAt
	finalExpiresAt := existing.ExpiresAt

	if v, ok := safeUpdates["starts_at"].(time.Time); ok {
		finalStartsAt = v
	}
	if v, ok := safeUpdates["expires_at"].(time.Time); ok {
		finalExpiresAt = v
	}
	if !finalExpiresAt.After(finalStartsAt) {
		return apperr.New(apperr.CodeBadRequest, "expiry date must be after start date")
	}

	result := s.db.Model(&models.Coupon{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(safeUpdates)

	if result.Error != nil {
		return fromDB(result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.New(apperr.CodeNotFound, "coupon not found")
	}

	return nil
}

func (s *Service) Delete(id uuid.UUID) error {
	result := s.db.Model(&models.Coupon{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", time.Now())

	if result.Error != nil {
		return fromDB(result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.New(apperr.CodeNotFound, "coupon not found")
	}

	return nil
}

func (s *Service) ValidateForPlan(code, plan string, originalAmount int64) (map[string]interface{}, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	plan = strings.ToLower(strings.TrimSpace(plan))

	if code == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "coupon code is required")
	}
	if plan == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "plan is required")
	}
	if originalAmount <= 0 {
		return nil, apperr.New(apperr.CodeBadRequest, "original amount must be greater than zero")
	}

	var c models.Coupon
	if err := s.db.Where("code = ? AND deleted_at IS NULL", code).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeBadRequest, "invalid or expired coupon code")
		}
		return nil, fromDB(err)
	}

	var redemptionCount int64
	if err := s.db.Model(&models.CouponRedemption{}).
		Where("coupon_id = ?", c.ID).
		Count(&redemptionCount).Error; err != nil {
		return nil, fromDB(err)
	}

	if !c.IsValid(int(redemptionCount)) {
		return nil, apperr.New(apperr.CodeBadRequest, "coupon is no longer valid")
	}
	if !c.IsPlanEligible(plan) {
		return nil, apperr.New(apperr.CodeBadRequest, "coupon not applicable to this plan")
	}

	discount := c.CalculateDiscount(originalAmount)
	finalAmount := originalAmount - discount

	return map[string]interface{}{
		"valid":               true,
		"coupon_code":         c.Code,
		"discount_percentage": c.DiscountValue,
		"original_amount":     originalAmount,
		"discount_amount":     discount,
		"final_amount":        finalAmount,
		"message":             "Coupon applied successfully",
	}, nil
}

func (s *Service) Redeem(
	couponID, businessID, subscriptionID uuid.UUID,
	plan string,
	original, discount, final int64,
	paystackRef, channel, ipAddress, userAgent string,
) error {
	redemption := models.CouponRedemption{
		CouponID:         couponID,
		BusinessID:       businessID,
		SubscriptionID:   subscriptionID,
		SubscriptionPlan: strings.ToLower(strings.TrimSpace(plan)),
		OriginalAmount:   original,
		DiscountAmount:   discount,
		FinalAmount:      final,
		PaymentReference: strings.TrimSpace(paystackRef),
		PaymentChannel:   strings.TrimSpace(channel),
		IPAddress:        net.ParseIP(ipAddress),
		UserAgent:        strings.TrimSpace(userAgent),
	}

	if err := s.db.Create(&redemption).Error; err != nil {
		return fromDB(err)
	}

	return nil
}
