package worker

import (
	"log"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type SubscriptionWorker struct {
	db *gorm.DB
}

func NewSubscriptionWorker(db *gorm.DB) *SubscriptionWorker {
	return &SubscriptionWorker{db: db}
}

func (w *SubscriptionWorker) RunExpiry()    { /* your original code unchanged */ }
func (w *SubscriptionWorker) RunReminders() { /* your original code unchanged */ }

func (w *SubscriptionWorker) CleanupPending() {
	log.Println("[SUBSCRIPTION] Running pending cleanup")
	result := w.db.Where("status = ? AND expires_at < ?", "pending", time.Now().Add(-24*time.Hour)).
		Delete(&models.PendingSubscription{})
	log.Printf("[SUBSCRIPTION] Cleaned %d stale pending records", result.RowsAffected)
}

func (w *SubscriptionWorker) notifyExpired(businessID interface{}, plan string) {
	// TODO: implement expiry notification (email/push)
	log.Printf("[SUBSCRIPTION] notifyExpired called for businessID=%v plan=%s", businessID, plan)
}

func (w *SubscriptionWorker) notifyExpiring(businessID interface{}, plan, renewalDate string) {
	// TODO: implement expiry reminder notification (email/push)
	log.Printf("[SUBSCRIPTION] notifyExpiring called for businessID=%v plan=%s renewalDate=%s", businessID, plan, renewalDate)
}

func (w *SubscriptionWorker) findOwner(businessID interface{}) (*models.User, error) {
	var user models.User
	if err := w.db.
		Joins("JOIN businesses ON businesses.owner_id = users.id").
		Where("businesses.id = ?", businessID).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
