package subscription

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/email"
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

// ─────────────────────────────────────────────────────────
// NEW METHODS FOR PAYSTACK SUBSCRIPTION PAYMENTS
// ─────────────────────────────────────────────────────────

func (s *Service) InitiatePayment(businessID uuid.UUID, plan string, periodMonths int, couponCode *string) (map[string]interface{}, error) {
	if plan != models.PlanGrowth && plan != models.PlanPro {
		return nil, errors.New("invalid plan: only growth and pro are supported")
	}

	basePrice := int64(0)
	if plan == models.PlanGrowth {
		basePrice = 500000 * int64(periodMonths)
	} else if plan == models.PlanPro {
		basePrice = 1000000 * int64(periodMonths)
	}

	finalAmount := basePrice
	var couponData map[string]interface{}

	if couponCode != nil && *couponCode != "" {
		var err error
		couponData, err = s.couponService.ValidateForPlan(*couponCode, plan, basePrice)
		if err != nil {
			return nil, err
		}
		finalAmount = couponData["final_amount"].(int64)
	}

	reference := fmt.Sprintf("sub_%s_%d", businessID.String()[:8], time.Now().UnixNano()/1e6)

	// 100% discount case
	if finalAmount == 0 {
		if _, err := s.Activate(businessID, plan, reference, periodMonths); err != nil {
			return nil, err
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

	// Normal payment
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

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return sub, nil
}

// ─────────────────────────────────────────────────────────
// YOUR ORIGINAL CODE (unchanged below)
// ─────────────────────────────────────────────────────────

func (s *Service) Get(businessID uuid.UUID) (*models.Subscription, error) {
	var sub models.Subscription
	if err := s.db.Where("business_id = ?", businessID).First(&sub).Error; err != nil {
		return nil, errors.New("subscription not found")
	}
	return &sub, nil
}

func (s *Service) Activate(businessID uuid.UUID, plan, paystackRef string, periodMonths int) (*models.Subscription, error) {
	if plan != models.PlanFree && plan != models.PlanGrowth && plan != models.PlanPro {
		return nil, errors.New("invalid plan")
	}

	limits := models.PlanLimits[plan]
	nowT := time.Now()
	renewsAt := nowT.AddDate(0, periodMonths, 0)
	now := &nowT
	renewsAtPtr := &renewsAt

	var sub models.Subscription
	err := s.db.Where("business_id = ?", businessID).First(&sub).Error

	if err != nil {
		sub = models.Subscription{
			BusinessID:               businessID,
			Plan:                     plan,
			Status:                   "active",
			CurrentPeriodStart:       now,
			CurrentPeriodEnd:         renewsAtPtr,
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
		"current_period_start":       now,
		"current_period_end":         renewsAtPtr,
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
	var subs []models.Subscription
	s.db.Where("status IN ? AND current_period_end < ?",
		[]string{"active", "grace_period", "cancelled"}, time.Now()).Find(&subs)

	freeLimits := models.PlanLimits[models.PlanFree]
	for _, sub := range subs {
		if sub.Plan == models.PlanFree {
			continue
		}

		var bu models.BusinessUser
		if err := s.db.Where("business_id = ? AND role = 'owner'", sub.BusinessID).First(&bu).Error; err != nil {
			continue
		}
		var owner models.User
		s.db.First(&owner, bu.UserID)

		s.db.Model(&sub).Updates(map[string]interface{}{
			"plan":          models.PlanFree,
			"status":        "active",
			"max_staff":     freeLimits.MaxStaff,
			"max_stores":    freeLimits.MaxStores,
			"max_products":  freeLimits.MaxProducts,
			"max_customers": freeLimits.MaxCustomers,
		})

		email.SendSubscriptionExpired(owner.Email, owner.FirstName, sub.Plan)
	}
	return nil
}

func (s *Service) SendExpiryReminders() {
	in3Days := time.Now().Add(72 * time.Hour)
	dayStart := time.Date(in3Days.Year(), in3Days.Month(), in3Days.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	var subs []models.Subscription
	s.db.Where("status = 'active' AND plan != ? AND current_period_end BETWEEN ? AND ?",
		models.PlanFree, dayStart, dayEnd).Find(&subs)

	for _, sub := range subs {
		var bu models.BusinessUser
		if err := s.db.Where("business_id = ? AND role = 'owner'", sub.BusinessID).First(&bu).Error; err != nil {
			continue
		}
		var owner models.User
		s.db.First(&owner, bu.UserID)

		if sub.CurrentPeriodEnd == nil {
			continue
		}
		renewalDate := sub.CurrentPeriodEnd.Format("2 January 2006")
		email.SendSubscriptionExpiring(owner.Email, owner.FirstName, sub.Plan, renewalDate)
	}
}
