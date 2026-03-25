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
	body, err := pkgPaystack.VerifySignature(c.Request, h.paystackSecret)
	if err != nil {
		log.Printf("Paystack webhook signature verification failed: %v", err)
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

func (h *Handler) processPaystackEvent(event *pkgPaystack.WebhookEvent) {
	switch event.Event {
	case pkgPaystack.EventChargeSuccess:
		if data, err := pkgPaystack.ParseChargeData(event.Data); err == nil {
			h.onChargeSuccess(data)
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
		log.Printf("[WEBHOOK] paystack unhandled event: %s", event.Event)
	}
}

func (h *Handler) Flutterwave(c *gin.Context) {
	// ... your existing flutterwave code
	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (h *Handler) onChargeSuccess(data *pkgPaystack.ChargeData) {
	paymentType, _ := data.Metadata["type"].(string)

	switch paymentType {
	case "digital_purchase":
		// your existing digital purchase logic

	case "subscription_upgrade":
		if _, err := h.subscriptionSvc.ActivateFromSuccessfulPayment(data.Reference); err != nil {
			log.Printf("[WEBHOOK] Failed to activate subscription for ref %s: %v", data.Reference, err)
		} else {
			log.Printf("[WEBHOOK] Subscription activated for reference: %s", data.Reference)
		}

	default:
		log.Printf("[WEBHOOK] unknown payment type: %q", paymentType)
	}
}
