// internal/worker/payout.go
package worker

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	"ogaos-backend/internal/pkg/email"
)

const (
	maxPayoutAttempts = 3
	payoutCooldown    = 24 * time.Hour // wait 24 h between retries
)

// PayoutWorker processes pending digital order payouts.
// Run once daily (or more frequently) via a cron job or ticker.
type PayoutWorker struct {
	db             *gorm.DB
	paystackClient *pkgPaystack.Client
}

func NewPayoutWorker(db *gorm.DB, paystackClient *pkgPaystack.Client) *PayoutWorker {
	return &PayoutWorker{db: db, paystackClient: paystackClient}
}

// Run processes all pending and retryable payouts.
func (w *PayoutWorker) Run() {
	log.Println("[PAYOUT] Starting payout run")

	var orders []models.DigitalOrder
	w.db.
		Where("payout_status IN ? AND payout_attempts < ?",
			[]string{models.PayoutStatusPending, models.PayoutStatusFailed},
			maxPayoutAttempts,
		).
		Where("created_at < ?", time.Now().Add(-10*time.Minute)). // 10-min settle buffer
		Preload("DigitalProduct").
		Preload("Business").
		Find(&orders)

	log.Printf("[PAYOUT] Found %d orders to process", len(orders))

	for _, order := range orders {
		if err := w.processOrder(order); err != nil {
			log.Printf("[PAYOUT] Order %s failed: %v", order.ID, err)
		}
	}

	log.Println("[PAYOUT] Run complete")
}

func (w *PayoutWorker) processOrder(order models.DigitalOrder) error {
	// Find default payout account for the business
	var payoutAccount models.BusinessPayoutAccount
	if err := w.db.Where("business_id = ? AND is_default = true AND is_verified = true", order.BusinessID).
		First(&payoutAccount).Error; err != nil {
		return w.failOrder(order, "no verified default payout account found")
	}

	if payoutAccount.PaystackRecipientCode == nil {
		return w.failOrder(order, "payout account has no Paystack recipient code")
	}

	// Mark as processing to prevent double-dispatch
	w.db.Model(&order).Updates(map[string]interface{}{
		"payout_status":   models.PayoutStatusProcessing,
		"payout_attempts": gorm.Expr("payout_attempts + 1"),
	})

	ref := fmt.Sprintf("payout-%s", order.ID)
	resp, err := w.paystackClient.InitiateTransfer(pkgPaystack.InitiateTransferRequest{
		Source:    "balance",
		Amount:    order.OwnerPayoutAmount,
		Recipient: *payoutAccount.PaystackRecipientCode,
		Reason:    fmt.Sprintf("Payout for %s", order.DigitalProduct.Title),
		Reference: ref,
	})
	if err != nil {
		return w.failOrder(order, err.Error())
	}

	// Paystack transfer is async — final status arrives via webhook
	// Mark as processing with the transfer reference so the webhook handler can reconcile
	w.db.Model(&order).Updates(map[string]interface{}{
		"payout_status":    models.PayoutStatusProcessing,
		"payout_reference": resp.Data.TransferCode,
	})

	log.Printf("[PAYOUT] Order %s: transfer initiated ref=%s", order.ID, resp.Data.TransferCode)
	return nil
}

// MarkPayoutComplete is called by the webhook handler when transfer.success fires.
func (w *PayoutWorker) MarkPayoutComplete(transferCode string) {
	now := time.Now()
	result := w.db.Model(&models.DigitalOrder{}).
		Where("payout_reference = ?", transferCode).
		Updates(map[string]interface{}{
			"payout_status":       models.PayoutStatusCompleted,
			"payout_completed_at": now,
		})
	if result.RowsAffected == 0 {
		log.Printf("[PAYOUT] MarkPayoutComplete: no order found for transfer_code=%s", transferCode)
		return
	}

	// Notify business owner
	var order models.DigitalOrder
	if err := w.db.Where("payout_reference = ?", transferCode).
		Preload("DigitalProduct").
		Preload("Business").
		First(&order).Error; err != nil {
		return
	}
	var bu models.BusinessUser
	if err := w.db.Where("business_id = ? AND role = 'owner'", order.BusinessID).First(&bu).Error; err != nil {
		return
	}
	var owner models.User
	if err := w.db.First(&owner, bu.UserID).Error; err != nil {
		return
	}
	email.SendPayoutNotification(
		owner.Email,
		owner.FirstName+" "+owner.LastName,
		order.DigitalProduct.Title,
		order.OwnerPayoutAmount,
	)
}

// MarkPayoutFailed is called by the webhook handler when transfer.failed / transfer.reversed fires.
func (w *PayoutWorker) MarkPayoutFailed(transferCode, reason string) {
	var order models.DigitalOrder
	if err := w.db.Where("payout_reference = ?", transferCode).
		Preload("DigitalProduct").
		Preload("Business").
		First(&order).Error; err != nil {
		log.Printf("[PAYOUT] MarkPayoutFailed: no order found for transfer_code=%s", transferCode)
		return
	}

	updates := map[string]interface{}{
		"payout_status":      models.PayoutStatusFailed,
		"payout_fail_reason": reason,
	}

	// If max attempts reached, give up — notify owner
	if order.PayoutAttempts >= maxPayoutAttempts {
		updates["payout_status"] = "exhausted"
		w.notifyPayoutFailed(order, reason)
	}
	// Otherwise leave as failed — Run() will retry on next cycle

	w.db.Model(&order).Updates(updates)
}

func (w *PayoutWorker) failOrder(order models.DigitalOrder, reason string) error {
	updates := map[string]interface{}{
		"payout_status":      models.PayoutStatusFailed,
		"payout_fail_reason": reason,
		"payout_attempts":    gorm.Expr("payout_attempts + 1"),
	}
	if order.PayoutAttempts+1 >= maxPayoutAttempts {
		updates["payout_status"] = "exhausted"
		w.notifyPayoutFailed(order, reason)
	}
	w.db.Model(&order).Updates(updates)
	return fmt.Errorf("payout failed for order %s: %s", order.ID, reason)
}

func (w *PayoutWorker) notifyPayoutFailed(order models.DigitalOrder, reason string) {
	var bu models.BusinessUser
	if err := w.db.Where("business_id = ? AND role = 'owner'", order.BusinessID).First(&bu).Error; err != nil {
		return
	}
	var owner models.User
	if err := w.db.First(&owner, bu.UserID).Error; err != nil {
		return
	}
	email.SendPayoutFailed(
		owner.Email,
		owner.FirstName+" "+owner.LastName,
		order.DigitalProduct.Title,
		order.OwnerPayoutAmount,
		reason,
	)
}
