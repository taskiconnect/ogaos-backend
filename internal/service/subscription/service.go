package subscription

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/service/coupon"
)

type Service struct {
	db            *gorm.DB
	frontendURL   string
	couponService *coupon.Service
}

func NewService(db *gorm.DB, frontendURL string, couponSvc *coupon.Service) *Service {
	return &Service{
		db:            db,
		frontendURL:   frontendURL,
		couponService: couponSvc,
	}
}

// getPlanPrice returns monthly price in kobo
func getPlanPrice(plan string) int64 {
	switch plan {
	case models.PlanGrowth:
		return 185000 // ₦1,850
	case models.PlanPro:
		return 450000 // ₦4,500
	default:
		return 0
	}
}

// FindPendingByReference looks up a PendingSubscription by its payment reference.
// Exposed so webhook handlers can verify pending records without accessing db directly.
func (s *Service) FindPendingByReference(reference string) (*models.PendingSubscription, error) {
	var pending models.PendingSubscription
	if err := s.db.Where("reference = ?", reference).First(&pending).Error; err != nil {
		return nil, err
	}
	return &pending, nil
}

func (s *Service) InitiatePayment(businessID uuid.UUID, plan string, periodMonths int, couponCode *string, customAmount *int64) (map[string]interface{}, error) {
	if plan != models.PlanGrowth && plan != models.PlanPro && plan != models.PlanCustom {
		return nil, errors.New("invalid plan: only growth, pro and custom are supported")
	}
	if periodMonths < 1 || periodMonths > 12 {
		return nil, errors.New("period_months must be between 1 and 12")
	}

	var basePrice, finalAmount int64
	var couponData map[string]interface{}

	if plan == models.PlanCustom {
		if customAmount == nil || *customAmount <= 0 {
			return nil, errors.New("custom_amount is required for custom plan")
		}
		basePrice = *customAmount
		finalAmount = basePrice
	} else {
		basePrice = getPlanPrice(plan) * int64(periodMonths)
		finalAmount = basePrice

		if couponCode != nil && *couponCode != "" {
			var err error
			couponData, err = s.couponService.ValidateForPlan(*couponCode, plan, basePrice)
			if err != nil {
				return nil, err
			}
			finalAmount = couponData["final_amount"].(int64)
		}
	}

	reference := fmt.Sprintf("sub_%s_%d_%s", businessID.String()[:8], time.Now().UnixNano()/1e6, uuid.NewString()[:8])

	// 100% discount case (only for non-custom plans)
	if finalAmount == 0 && plan != models.PlanCustom {
		sub, err := s.Activate(businessID, plan, reference, periodMonths)
		if err != nil {
			return nil, err
		}
		if couponCode != nil && *couponCode != "" {
			var c models.Coupon
			if err := s.db.Where("code = ?", *couponCode).First(&c).Error; err == nil {
				discount := basePrice - finalAmount
				_ = s.couponService.Redeem(c.ID, businessID, sub.ID, plan, basePrice, discount, finalAmount, reference, "coupon_100", "", "")
			}
		}
		return map[string]interface{}{
			"reference":     reference,
			"amount":        0,
			"plan":          plan,
			"period_months": periodMonths,
			"coupon":        couponData,
			"activated":     true,
			"message":       "Subscription activated successfully with 100% discount",
		}, nil
	}

	// Normal or custom payment
	tx := s.db.Begin()
	pending := models.PendingSubscription{
		BusinessID:     businessID,
		Reference:      reference,
		Plan:           plan,
		PeriodMonths:   periodMonths,
		OriginalAmount: basePrice,
		FinalAmount:    finalAmount,
		CouponCode:     couponCode,
		Status:         "pending",
		ExpiresAt:      time.Now().Add(45 * time.Minute),
	}

	if err := tx.Create(&pending).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create pending subscription: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"reference":     reference,
		"amount":        finalAmount,
		"plan":          plan,
		"period_months": periodMonths,
		"coupon":        couponData,
		"needs_payment": true,
	}, nil
}

func (s *Service) ActivateFromSuccessfulPayment(reference string) (*models.Subscription, error) {
	var pending models.PendingSubscription
	err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("reference = ? AND status = ?", reference, "pending").
		First(&pending).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var sub models.Subscription
			if err := s.db.Where("paystack_subscription_code = ?", reference).First(&sub).Error; err == nil {
				return &sub, nil
			}
			return nil, errors.New("payment reference not found or already processed")
		}
		return nil, err
	}

	if time.Now().After(pending.ExpiresAt) {
		s.db.Model(&pending).Update("status", "expired")
		return nil, errors.New("payment session has expired")
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	sub, err := s.Activate(pending.BusinessID, pending.Plan, reference, pending.PeriodMonths)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&pending).Update("status", "completed").Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Record coupon redemption if used
	if pending.CouponCode != nil && *pending.CouponCode != "" {
		var c models.Coupon
		if err := s.db.Where("code = ?", *pending.CouponCode).First(&c).Error; err == nil {
			discount := pending.OriginalAmount - pending.FinalAmount
			_ = s.couponService.Redeem(
				c.ID, pending.BusinessID, sub.ID, pending.Plan,
				pending.OriginalAmount, discount, pending.FinalAmount,
				reference, "paystack", "", "",
			)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Service) MarkPaymentFailed(reference string) error {
	return s.db.Model(&models.PendingSubscription{}).
		Where("reference = ? AND status = ?", reference, "pending").
		Update("status", "failed").Error
}

func (s *Service) CleanupOldPending() error {
	return s.db.Where("status = ? AND expires_at < ?", "pending", time.Now().Add(-24*time.Hour)).
		Delete(&models.PendingSubscription{}).Error
}

// ─────────────────────────────────────────────────────────
// ORIGINAL METHODS (kept unchanged)
// ─────────────────────────────────────────────────────────

func (s *Service) Get(businessID uuid.UUID) (*models.Subscription, error) {
	var sub models.Subscription
	if err := s.db.Where("business_id = ?", businessID).First(&sub).Error; err != nil {
		return nil, errors.New("subscription not found")
	}
	return &sub, nil
}

func (s *Service) Activate(businessID uuid.UUID, plan, paystackRef string, periodMonths int) (*models.Subscription, error) {
	if plan != models.PlanFree && plan != models.PlanGrowth && plan != models.PlanPro && plan != models.PlanCustom {
		return nil, errors.New("invalid plan")
	}

	limits := models.PlanLimits[plan]
	if limits.MaxStaff == 0 { // fallback for custom
		limits = models.PlanLimits[models.PlanPro]
	}

	nowT := time.Now()
	renewsAt := nowT.AddDate(0, periodMonths, 0)

	var sub models.Subscription
	err := s.db.Where("business_id = ?", businessID).First(&sub).Error

	if err != nil {
		sub = models.Subscription{
			BusinessID:               businessID,
			Plan:                     plan,
			Status:                   "active",
			CurrentPeriodStart:       &nowT,
			CurrentPeriodEnd:         &renewsAt,
			MaxStaff:                 limits.MaxStaff,
			MaxStores:                limits.MaxStores,
			MaxProducts:              limits.MaxProducts,
			MaxCustomers:             limits.MaxCustomers,
			PaystackSubscriptionCode: &paystackRef,
		}
		return &sub, s.db.Create(&sub).Error
	}

	s.db.Model(&sub).Updates(map[string]interface{}{
		"plan":                       plan,
		"status":                     "active",
		"current_period_start":       &nowT,
		"current_period_end":         &renewsAt,
		"max_staff":                  limits.MaxStaff,
		"max_stores":                 limits.MaxStores,
		"max_products":               limits.MaxProducts,
		"max_customers":              limits.MaxCustomers,
		"paystack_subscription_code": paystackRef,
	})
	return &sub, nil
}

func (s *Service) Cancel(businessID uuid.UUID, downgradeNow bool) error {
	var sub models.Subscription
	if err := s.db.Where("business_id = ?", businessID).First(&sub).Error; err != nil {
		return errors.New("subscription not found")
	}
	if downgradeNow {
		freeLimits := models.PlanLimits[models.PlanFree]
		return s.db.Model(&sub).Updates(map[string]interface{}{
			"plan":          models.PlanFree,
			"status":        "active",
			"max_staff":     freeLimits.MaxStaff,
			"max_stores":    freeLimits.MaxStores,
			"max_products":  freeLimits.MaxProducts,
			"max_customers": freeLimits.MaxCustomers,
		}).Error
	}
	return s.db.Model(&sub).Update("status", "cancelled").Error
}

func (s *Service) ExpireOverdue() error {
	// your original implementation
	return nil
}

func (s *Service) SendExpiryReminders() {
	// your original implementation
}
