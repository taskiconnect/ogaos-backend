// internal/service/subscription/service.go
package subscription

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/email"
)

type Service struct {
	db          *gorm.DB
	frontendURL string
}

func NewService(db *gorm.DB, frontendURL string) *Service {
	return &Service{db: db, frontendURL: frontendURL}
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// Get returns the current subscription for a business.
func (s *Service) Get(businessID uuid.UUID) (*models.Subscription, error) {
	var sub models.Subscription
	if err := s.db.Where("business_id = ?", businessID).First(&sub).Error; err != nil {
		return nil, errors.New("subscription not found")
	}
	return &sub, nil
}

// Activate upgrades or activates a subscription after a successful payment.
// Called from the Paystack webhook handler on charge.success.
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
		// Create new subscription
		sub = models.Subscription{
			BusinessID:         businessID,
			Plan:               plan,
			Status:             "active",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   renewsAtPtr,
			MaxStaff:           limits.MaxStaff,
			MaxStores:          limits.MaxStores,
			MaxProducts:        limits.MaxProducts,
			MaxCustomers:       limits.MaxCustomers,
		}
		return &sub, s.db.Create(&sub).Error
	}

	// Update existing
	s.db.Model(&sub).Updates(map[string]interface{}{
		"plan":                 plan,
		"status":               "active",
		"current_period_start": now,
		"current_period_end":   renewsAtPtr,
		"max_staff":            limits.MaxStaff,
		"max_stores":           limits.MaxStores,
		"max_products":         limits.MaxProducts,
		"max_customers":        limits.MaxCustomers,
	})
	return &sub, nil
}

// Cancel downgrades a subscription to free plan at period end.
// For immediate cancellation pass downgradeNow=true.
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
	// Mark as cancelled — access continues until period end
	return s.db.Model(&sub).Update("status", "cancelled").Error
}

// ExpireOverdue checks all subscriptions and downgrades those past period end.
// Called daily by a scheduled job.
func (s *Service) ExpireOverdue() error {
	var subs []models.Subscription
	s.db.Where("status IN ? AND current_period_end < ?",
		[]string{"active", "grace_period", "cancelled"}, time.Now()).Find(&subs)

	freeLimits := models.PlanLimits[models.PlanFree]
	for _, sub := range subs {
		if sub.Plan == models.PlanFree {
			continue
		}

		// Load business owner email
		var bu models.BusinessUser
		if err := s.db.Where("business_id = ? AND role = 'owner'", sub.BusinessID).First(&bu).Error; err != nil {
			continue
		}
		var owner models.User
		s.db.First(&owner, bu.UserID)

		var business models.Business
		s.db.First(&business, sub.BusinessID)

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

// SendExpiryReminders emails owners whose subscription renews in 3 days.
// Called daily by a scheduled job.
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
