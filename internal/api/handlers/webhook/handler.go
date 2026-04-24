package webhook

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	pkgPaystack "ogaos-backend/internal/external/paystack"
	svcDigital "ogaos-backend/internal/service/digital"
	"ogaos-backend/internal/service/subscription"
	"ogaos-backend/internal/worker"
)

type Handler struct {
	paystackSecret  string
	flutterwaveHash string
	digitalSvc      *svcDigital.Service
	subscriptionSvc *subscription.Service
	payoutWorker    *worker.PayoutWorker
}

func NewHandler(
	paystackSecret string,
	flutterwaveHash string,
	digitalSvc *svcDigital.Service,
	subscriptionSvc *subscription.Service,
	payoutWorker *worker.PayoutWorker,
) *Handler {
	return &Handler{
		paystackSecret:  paystackSecret,
		flutterwaveHash: flutterwaveHash,
		digitalSvc:      digitalSvc,
		subscriptionSvc: subscriptionSvc,
		payoutWorker:    payoutWorker,
	}
}

func (h *Handler) Paystack(c *gin.Context) {
	allowedIPs := []string{"52.31.139.75", "52.49.173.169", "52.214.14.220"}
	if !contains(allowedIPs, c.ClientIP()) {
		log.Printf("[WEBHOOK] Blocked non-Paystack IP: %s", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"success": false})
		return
	}

	body, err := pkgPaystack.VerifySignature(c.Request, h.paystackSecret)
	if err != nil {
		log.Printf("[WEBHOOK] Paystack webhook signature failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid signature"})
		return
	}

	event, err := pkgPaystack.ParseEvent(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid payload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})

	go h.processPaystackEvent(event)
}

func (h *Handler) Flutterwave(c *gin.Context) {
	receivedHash := c.GetHeader("verif-hash")
	if receivedHash != h.flutterwaveHash {
		log.Printf("[WEBHOOK] Flutterwave: invalid hash from IP %s", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "invalid signature"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
	log.Printf("[WEBHOOK] Flutterwave event received — processing not yet implemented")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (h *Handler) processPaystackEvent(event *pkgPaystack.WebhookEvent) {
	switch event.Event {
	case pkgPaystack.EventChargeSuccess:
		if data, err := pkgPaystack.ParseChargeData(event.Data); err == nil {
			h.onChargeSuccess(data)
		} else {
			log.Printf("[WEBHOOK] failed to parse charge.success: %v", err)
		}

	case "charge.failed":
		if data, err := pkgPaystack.ParseChargeData(event.Data); err == nil {
			paymentType, _ := data.Metadata["type"].(string)
			if strings.TrimSpace(paymentType) == "subscription_upgrade" {
				if err := h.subscriptionSvc.MarkPaymentFailed(data.Reference); err != nil {
					log.Printf("[WEBHOOK] mark failed error ref=%s: %v", data.Reference, err)
				}
			}
		}

	case pkgPaystack.EventTransferSuccess:
		if data, err := pkgPaystack.ParseTransferData(event.Data); err == nil {
			h.payoutWorker.MarkPayoutComplete(data.TransferCode)
		} else {
			log.Printf("[WEBHOOK] failed to parse transfer.success: %v", err)
		}

	case pkgPaystack.EventTransferFailed, pkgPaystack.EventTransferReversed:
		if data, err := pkgPaystack.ParseTransferData(event.Data); err == nil {
			reason := strings.TrimSpace(data.FailureReason)
			if reason == "" {
				reason = strings.TrimSpace(data.Status)
			}
			if reason == "" {
				reason = "transfer failed"
			}
			h.payoutWorker.MarkPayoutFailed(data.TransferCode, reason)
		} else {
			log.Printf("[WEBHOOK] failed to parse transfer failure event: %v", err)
		}

	default:
		log.Printf("[WEBHOOK] unhandled event: %s", event.Event)
	}
}

func (h *Handler) onChargeSuccess(data *pkgPaystack.ChargeData) {
	if data == nil || strings.TrimSpace(data.Status) != "success" {
		return
	}

	paymentType, _ := data.Metadata["type"].(string)
	paymentType = strings.TrimSpace(paymentType)

	switch paymentType {
	case "digital_product":
		if err := h.digitalSvc.MarkOrderPaidByReference(data.Reference); err != nil {
			log.Printf("[WEBHOOK] digital order finalization failed ref=%s: %v", data.Reference, err)
			return
		}
		log.Printf("[WEBHOOK] Digital order finalized ref=%s", data.Reference)

	case "subscription_upgrade":
		pending, err := h.subscriptionSvc.FindPendingByReference(data.Reference)
		if err != nil {
			log.Printf("[SECURITY] pending not found ref=%s", data.Reference)
			return
		}

		if int64(data.Amount) != pending.FinalAmount {
			log.Printf("[SECURITY] amount mismatch! webhook=%d expected=%d ref=%s", data.Amount, pending.FinalAmount, data.Reference)
			return
		}

		businessIDFromMeta, _ := data.Metadata["business_id"].(string)
		if businessIDFromMeta != pending.BusinessID.String() {
			log.Printf("[SECURITY] business_id mismatch ref=%s", data.Reference)
			return
		}

		if _, err := h.subscriptionSvc.ActivateFromSuccessfulPayment(data.Reference); err != nil {
			log.Printf("[WEBHOOK] subscription activation failed ref=%s: %v", data.Reference, err)
		} else {
			log.Printf("[WEBHOOK] Subscription activated ref=%s", data.Reference)
		}

	default:
		log.Printf("[WEBHOOK] charge.success with unknown payment type ref=%s type=%s", data.Reference, paymentType)
	}
}
