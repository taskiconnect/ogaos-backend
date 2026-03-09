// internal/worker/subscription.go
package worker

import (
	"log"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/email"
)

// SubscriptionWorker handles subscription lifecycle events.
// Run daily via a ticker or cron.
type SubscriptionWorker struct {
	db *gorm.DB
}

func NewSubscriptionWorker(db *gorm.DB) *SubscriptionWorker {
	return &SubscriptionWorker{db: db}
}

// RunExpiry downgrades all subscriptions whose current_period_end has passed.
func (w *SubscriptionWorker) RunExpiry() {
	log.Println("[SUBSCRIPTION] Running expiry check")

	var subs []models.Subscription
	w.db.Where(
		"status IN ? AND current_period_end < ? AND plan != ?",
		[]string{"active", "cancelled", "grace_period"},
		time.Now(),
		models.PlanFree,
	).Find(&subs)

	log.Printf("[SUBSCRIPTION] Found %d expired subscriptions", len(subs))

	freeLimits := models.PlanLimits[models.PlanFree]
	for _, sub := range subs {
		oldPlan := sub.Plan

		w.db.Model(&sub).Updates(map[string]interface{}{
			"plan":          models.PlanFree,
			"status":        "active",
			"max_staff":     freeLimits.MaxStaff,
			"max_stores":    freeLimits.MaxStores,
			"max_products":  freeLimits.MaxProducts,
			"max_customers": freeLimits.MaxCustomers,
		})

		w.notifyExpired(sub.BusinessID, oldPlan)
	}

	log.Println("[SUBSCRIPTION] Expiry run complete")
}

// RunReminders emails owners whose subscription renews in exactly 3 days.
func (w *SubscriptionWorker) RunReminders() {
	log.Println("[SUBSCRIPTION] Running renewal reminders")

	// Target: subscriptions ending tomorrow+48h window (i.e. in ~3 days)
	in3Days := time.Now().Add(72 * time.Hour)
	windowStart := time.Date(in3Days.Year(), in3Days.Month(), in3Days.Day(), 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(24 * time.Hour)

	var subs []models.Subscription
	w.db.Where(
		"status = 'active' AND plan != ? AND current_period_end BETWEEN ? AND ?",
		models.PlanFree, windowStart, windowEnd,
	).Find(&subs)

	log.Printf("[SUBSCRIPTION] Sending %d renewal reminders", len(subs))

	for _, sub := range subs {
		if sub.CurrentPeriodEnd == nil {
			continue
		}
		renewalDate := sub.CurrentPeriodEnd.Format("2 January 2006")
		w.notifyExpiring(sub.BusinessID, sub.Plan, renewalDate)
	}

	log.Println("[SUBSCRIPTION] Reminders run complete")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (w *SubscriptionWorker) notifyExpired(businessID interface{}, plan string) {
	owner, err := w.findOwner(businessID)
	if err != nil {
		return
	}
	email.SendSubscriptionExpired(owner.Email, owner.FirstName, plan)
}

func (w *SubscriptionWorker) notifyExpiring(businessID interface{}, plan, renewalDate string) {
	owner, err := w.findOwner(businessID)
	if err != nil {
		return
	}
	email.SendSubscriptionExpiring(owner.Email, owner.FirstName, plan, renewalDate)
}

func (w *SubscriptionWorker) findOwner(businessID interface{}) (*models.User, error) {
	var bu models.BusinessUser
	if err := w.db.Where("business_id = ? AND role = 'owner'", businessID).First(&bu).Error; err != nil {
		return nil, err
	}
	var owner models.User
	if err := w.db.First(&owner, bu.UserID).Error; err != nil {
		return nil, err
	}
	return &owner, nil
}
