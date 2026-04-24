// internal/worker/payout.go
package worker

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	"ogaos-backend/internal/pkg/email"
)

const (
	maxPayoutAttempts = 3
	payoutCooldown    = 24 * time.Hour
)

type PayoutWorker struct {
	db             *gorm.DB
	paystackClient *pkgPaystack.Client
}

func NewPayoutWorker(db *gorm.DB, paystackClient *pkgPaystack.Client) *PayoutWorker {
	return &PayoutWorker{db: db, paystackClient: paystackClient}
}

func (w *PayoutWorker) Run() {
	log.Println("[PAYOUT] Starting payout run")

	var orders []models.DigitalOrder
	w.db.
		Where("payout_status IN ? AND payout_attempts < ?",
			[]string{models.PayoutStatusPending, models.PayoutStatusFailed},
			maxPayoutAttempts,
		).
		Where("payment_status = ?", models.OrderPaymentStatusSuccessful).
		Where("created_at < ?", time.Now().Add(-10*time.Minute)).
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
	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return fmt.Errorf("order %s payment is not successful", order.ID)
	}

	if order.OwnerPayoutAmount <= 0 {
		return fmt.Errorf("order %s owner payout amount is invalid", order.ID)
	}

	var payoutAccount models.BusinessPayoutAccount
	if err := w.db.
		Where("business_id = ? AND is_default = true AND is_verified = true", order.BusinessID).
		First(&payoutAccount).Error; err != nil {
		return w.failOrder(order, "no verified default payout account found")
	}

	if payoutAccount.PaystackRecipientCode == nil || strings.TrimSpace(*payoutAccount.PaystackRecipientCode) == "" {
		return w.failOrder(order, "payout account has no Paystack recipient code")
	}

	w.db.Model(&order).Updates(map[string]interface{}{
		"payout_status":   models.PayoutStatusProcessing,
		"payout_attempts": gorm.Expr("payout_attempts + 1"),
	})

	ref := fmt.Sprintf("payout-%s", order.ID.String())
	resp, err := w.paystackClient.InitiateTransfer(pkgPaystack.InitiateTransferRequest{
		Source:    "balance",
		Amount:    order.OwnerPayoutAmount,
		Recipient: strings.TrimSpace(*payoutAccount.PaystackRecipientCode),
		Reason:    fmt.Sprintf("Payout for %s", order.DigitalProduct.Title),
		Reference: ref,
	})
	if err != nil {
		return w.failOrder(order, err.Error())
	}

	transferRef := strings.TrimSpace(resp.Data.TransferCode)
	if transferRef == "" {
		transferRef = strings.TrimSpace(resp.Data.Reference)
	}
	if transferRef == "" {
		transferRef = ref
	}

	w.db.Model(&order).Updates(map[string]interface{}{
		"payout_status":    models.PayoutStatusProcessing,
		"payout_reference": transferRef,
	})

	log.Printf("[PAYOUT] Order %s: transfer initiated ref=%s", order.ID, transferRef)
	return nil
}

func (w *PayoutWorker) MarkPayoutComplete(transferCode string) {
	now := time.Now()

	result := w.db.Model(&models.DigitalOrder{}).
		Where("payout_reference = ?", transferCode).
		Updates(map[string]interface{}{
			"payout_status":       models.PayoutStatusCompleted,
			"payout_completed_at": now,
			"payout_fail_reason":  nil,
		})
	if result.RowsAffected == 0 {
		log.Printf("[PAYOUT] MarkPayoutComplete: no order found for transfer_code=%s", transferCode)
		return
	}

	var order models.DigitalOrder
	if err := w.db.
		Where("payout_reference = ?", transferCode).
		Preload("DigitalProduct").
		Preload("Business").
		First(&order).Error; err != nil {
		return
	}

	var bu models.BusinessUser
	if err := w.db.
		Where("business_id = ? AND role = ? AND is_active = ?", order.BusinessID, "owner", true).
		First(&bu).Error; err != nil {
		return
	}

	var owner models.User
	if err := w.db.First(&owner, bu.UserID).Error; err != nil {
		return
	}

	name := strings.TrimSpace(owner.FirstName + " " + owner.LastName)
	if name == "" {
		name = owner.FirstName
	}

	email.SendPayoutNotification(
		owner.Email,
		name,
		order.DigitalProduct.Title,
		order.OwnerPayoutAmount,
	)
}

func (w *PayoutWorker) MarkPayoutFailed(transferCode, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "transfer failed"
	}

	var order models.DigitalOrder
	if err := w.db.
		Where("payout_reference = ?", transferCode).
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

	if order.PayoutAttempts >= maxPayoutAttempts {
		updates["payout_status"] = "exhausted"
		w.notifyPayoutFailed(order, reason)
	}

	w.db.Model(&order).Updates(updates)
}

func (w *PayoutWorker) HandleTransferSuccess(reference string) error {
	w.MarkPayoutComplete(reference)
	return nil
}

func (w *PayoutWorker) HandleTransferFailed(reference string, reason string) error {
	w.MarkPayoutFailed(reference, reason)
	return nil
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
	if err := w.db.
		Where("business_id = ? AND role = ? AND is_active = ?", order.BusinessID, "owner", true).
		First(&bu).Error; err != nil {
		return
	}

	var owner models.User
	if err := w.db.First(&owner, bu.UserID).Error; err != nil {
		return
	}

	name := strings.TrimSpace(owner.FirstName + " " + owner.LastName)
	if name == "" {
		name = owner.FirstName
	}

	email.SendPayoutFailed(
		owner.Email,
		name,
		order.DigitalProduct.Title,
		order.OwnerPayoutAmount,
		reason,
	)
}
