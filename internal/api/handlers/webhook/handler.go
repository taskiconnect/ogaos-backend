// internal/api/handlers/webhook/handler.go
package webhook

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	pkgFlutterwave "ogaos-backend/internal/external/flutterwave"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	svcDigital "ogaos-backend/internal/service/digital"
	svcSubscription "ogaos-backend/internal/service/subscription"
	"ogaos-backend/internal/worker"
)

type Handler struct {
	paystackSecret  string
	flutterwaveHash string
	digitalSvc      *svcDigital.Service
	subscriptionSvc *svcSubscription.Service
	payoutWorker    *worker.PayoutWorker
}

func NewHandler(
	paystackSecret string,
	flutterwaveHash string,
	digitalSvc *svcDigital.Service,
	subscriptionSvc *svcSubscription.Service,
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

// POST /webhooks/paystack
func (h *Handler) Paystack(c *gin.Context) {
	body, err := pkgPaystack.VerifySignature(c.Request, h.paystackSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid signature"})
		return
	}
	event, err := pkgPaystack.ParseEvent(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid payload"})
		return
	}

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

	case pkgPaystack.EventSubscriptionCreate:
		if data, err := pkgPaystack.ParseSubscriptionData(event.Data); err == nil {
			log.Printf("[WEBHOOK] subscription created code=%s plan=%s", data.SubscriptionCode, data.Plan.Name)
		}

	case pkgPaystack.EventSubscriptionDisable:
		if data, err := pkgPaystack.ParseSubscriptionData(event.Data); err == nil {
			log.Printf("[WEBHOOK] subscription disabled code=%s", data.SubscriptionCode)
		}

	default:
		log.Printf("[WEBHOOK] paystack unhandled event: %s", event.Event)
	}

	// Always 200 — stops Paystack from retrying
	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// POST /webhooks/flutterwave
func (h *Handler) Flutterwave(c *gin.Context) {
	body, err := pkgFlutterwave.VerifySignature(c.Request, h.flutterwaveHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid signature"})
		return
	}
	event, err := pkgFlutterwave.ParseEvent(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid payload"})
		return
	}

	switch event.Event {
	case pkgFlutterwave.EventChargeCompleted:
		if data, err := pkgFlutterwave.ParseChargeData(event.Data); err == nil && data.Status == "successful" {
			h.onFlutterwaveCharge(data)
		}
	default:
		log.Printf("[WEBHOOK] flutterwave unhandled event: %s", event.Event)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// ─── Event processors ─────────────────────────────────────────────────────────

func (h *Handler) onChargeSuccess(data *pkgPaystack.ChargeData) {
	paymentType, _ := data.Metadata["type"].(string)

	switch paymentType {
	case "digital_purchase":
		productID, err := metaUUID(data.Metadata, "product_id")
		if err != nil {
			log.Printf("[WEBHOOK] digital_purchase missing product_id: %v", err)
			return
		}
		buyerName, _ := data.Metadata["buyer_name"].(string)
		if _, err := h.digitalSvc.CompletePurchase(productID, svcDigital.PurchaseRequest{
			BuyerName:  buyerName,
			BuyerEmail: data.Customer.Email,
			Reference:  data.Reference,
			Channel:    "paystack",
		}); err != nil {
			log.Printf("[WEBHOOK] CompletePurchase error: %v", err)
		}

	case "subscription":
		businessID, err := metaUUID(data.Metadata, "business_id")
		if err != nil {
			log.Printf("[WEBHOOK] subscription missing business_id: %v", err)
			return
		}
		plan, _ := data.Metadata["plan"].(string)
		if _, err := h.subscriptionSvc.Activate(businessID, plan, data.Reference, 1); err != nil {
			log.Printf("[WEBHOOK] Activate subscription error: %v", err)
		}

	default:
		log.Printf("[WEBHOOK] charge.success unknown payment type: %q", paymentType)
	}
}

func (h *Handler) onFlutterwaveCharge(data *pkgFlutterwave.ChargeData) {
	if data.Meta == nil {
		return
	}
	productID, err := metaUUID(data.Meta, "product_id")
	if err != nil {
		log.Printf("[WEBHOOK] flutterwave missing product_id: %v", err)
		return
	}
	buyerName, _ := data.Meta["buyer_name"].(string)
	if _, err := h.digitalSvc.CompletePurchase(productID, svcDigital.PurchaseRequest{
		BuyerName:  buyerName,
		BuyerEmail: data.Customer.Email,
		Reference:  data.TxRef,
		Channel:    "flutterwave",
	}); err != nil {
		log.Printf("[WEBHOOK] flutterwave CompletePurchase error: %v", err)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func metaUUID(meta map[string]interface{}, key string) (uuid.UUID, error) {
	s, _ := meta[key].(string)
	return uuid.Parse(s)
}
