package coupon

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Create creates a new coupon (admin only)
func (s *Service) Create(c *models.Coupon) error {
	if c.Code = strings.ToUpper(strings.TrimSpace(c.Code)); c.Code == "" {
		return errors.New("coupon code is required")
	}
	return s.db.Create(c).Error
}

// List returns all active coupons for admin (with redemption count)
func (s *Service) List() ([]map[string]interface{}, error) {
	var coupons []models.Coupon
	if err := s.db.Where("deleted_at IS NULL").Find(&coupons).Error; err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(coupons))
	for i, c := range coupons {
		var count int64
		s.db.Model(&models.CouponRedemption{}).Where("coupon_id = ?", c.ID).Count(&count)

		result[i] = map[string]interface{}{
			"id":               c.ID,
			"code":             c.Code,
			"description":      c.Description,
			"discount_value":   c.DiscountValue,
			"applicable_plans": c.ApplicablePlans,
			"starts_at":        c.StartsAt,
			"expires_at":       c.ExpiresAt,
			"max_redemptions":  c.MaxRedemptions,
			"redemptions":      count,
			"remaining":        c.MaxRedemptions - int(count),
			"is_active":        c.IsActive,
			"created_at":       c.CreatedAt,
		}
	}
	return result, nil
}

// Get returns a single coupon by ID
func (s *Service) Get(id uuid.UUID) (*models.Coupon, error) {
	var c models.Coupon
	if err := s.db.Where("deleted_at IS NULL").First(&c, id).Error; err != nil {
		return nil, errors.New("coupon not found")
	}
	return &c, nil
}

// Update updates a coupon
func (s *Service) Update(id uuid.UUID, updates map[string]interface{}) error {
	return s.db.Model(&models.Coupon{}).Where("id = ? AND deleted_at IS NULL", id).Updates(updates).Error
}

// Delete soft-deletes a coupon
func (s *Service) Delete(id uuid.UUID) error {
	return s.db.Model(&models.Coupon{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// ValidateForPlan validates a coupon for a specific plan and returns discount info
func (s *Service) ValidateForPlan(code, plan string, originalAmount int64) (map[string]interface{}, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var c models.Coupon
	if err := s.db.Where("code = ? AND deleted_at IS NULL", code).First(&c).Error; err != nil {
		return nil, errors.New("invalid or expired coupon code")
	}

	// Count current redemptions
	var redemptionCount int64
	s.db.Model(&models.CouponRedemption{}).Where("coupon_id = ?", c.ID).Count(&redemptionCount)

	if !c.IsValid(int(redemptionCount)) {
		return nil, errors.New("coupon is no longer valid (expired or max uses reached)")
	}
	if !c.IsPlanEligible(plan) {
		return nil, errors.New("coupon not applicable to this plan")
	}

	discount := c.CalculateDiscount(originalAmount)
	final := originalAmount - discount

	return map[string]interface{}{
		"valid":               true,
		"coupon_code":         c.Code,
		"discount_percentage": c.DiscountValue,
		"original_amount":     originalAmount,
		"discount_amount":     discount,
		"final_amount":        final,
		"message":             "Coupon applied successfully!",
	}, nil
}

// Redeem records the redemption (called from subscription activation on successful payment)
func (s *Service) Redeem(couponID, businessID, subscriptionID uuid.UUID, plan string, original, discount, final int64, paystackRef, channel string) error {
	redemption := models.CouponRedemption{
		CouponID:         couponID,
		BusinessID:       businessID,
		SubscriptionID:   subscriptionID,
		SubscriptionPlan: plan,
		OriginalAmount:   original,
		DiscountAmount:   discount,
		FinalAmount:      final,
		PaymentReference: paystackRef,
		PaymentChannel:   channel,
	}
	return s.db.Create(&redemption).Error
}
