package webhook

import (
	"log"
	"net/http"

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
	// Paystack official webhook IPs (production)
	allowedIPs := []string{"52.31.139.75", "52.49.173.169", "52.214.14.220"}
	if !contains(allowedIPs, c.ClientIP()) {
		log.Printf("[WEBHOOK] Blocked non-Paystack IP: %s", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"success": false})
		return
	}

	body, err := pkgPaystack.VerifySignature(c.Request, h.paystackSecret)
	if err != nil {
		log.Printf("Paystack webhook signature failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid signature"})
		return
	}

	event, err := pkgPaystack.ParseEvent(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid payload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})

	// Pass no context — HTTP response is already written before goroutine runs
	go h.processPaystackEvent(event)
}

// Flutterwave handles incoming Flutterwave webhook events.
// TODO: implement full Flutterwave webhook verification and event processing.
func (h *Handler) Flutterwave(c *gin.Context) {
	receivedHash := c.GetHeader("verif-hash")
	if receivedHash != h.flutterwaveHash {
		log.Printf("[WEBHOOK] Flutterwave: invalid hash from IP %s", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "invalid signature"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})

	// TODO: parse event body and handle payment events
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

// processPaystackEvent handles the webhook event asynchronously.
// No *gin.Context — the HTTP response is already sent before this runs.
func (h *Handler) processPaystackEvent(event *pkgPaystack.WebhookEvent) {
	switch event.Event {
	case pkgPaystack.EventChargeSuccess:
		if data, err := pkgPaystack.ParseChargeData(event.Data); err == nil {
			h.onChargeSuccess(data)
		}
	case "charge.failed":
		if data, err := pkgPaystack.ParseChargeData(event.Data); err == nil {
			if err := h.subscriptionSvc.MarkPaymentFailed(data.Reference); err != nil {
				log.Printf("[WEBHOOK] mark failed error ref=%s: %v", data.Reference, err)
			}
		}
	case pkgPaystack.EventTransferSuccess:
		if data, err := pkgPaystack.ParseTransferData(event.Data); err == nil {
			h.payoutWorker.MarkPayoutComplete(data.TransferCode)
		}
	case pkgPaystack.EventTransferFailed, pkgPaystack.EventTransferReversed:
		if data, err := pkgPaystack.ParseTransferData(event.Data); err == nil {
			h.payoutWorker.MarkPayoutFailed(data.TransferCode, data.FailureReason)
		}
	default:
		log.Printf("[WEBHOOK] unhandled event: %s", event.Event)
	}
}

func (h *Handler) onChargeSuccess(data *pkgPaystack.ChargeData) {
	if data.Status != "success" {
		return
	}

	paymentType, _ := data.Metadata["type"].(string)
	if paymentType != "subscription_upgrade" {
		return
	}

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
		log.Printf("[WEBHOOK] activation failed ref=%s: %v", data.Reference, err)
	} else {
		log.Printf("[WEBHOOK] Subscription activated ref=%s", data.Reference)
	}
}
