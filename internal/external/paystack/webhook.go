// internal/external/paystack/webhook.go
package paystack

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Paystack webhook event types we handle
const (
	EventChargeSuccess        = "charge.success"
	EventTransferSuccess      = "transfer.success"
	EventTransferFailed       = "transfer.failed"
	EventTransferReversed     = "transfer.reversed"
	EventSubscriptionCreate   = "subscription.create"
	EventSubscriptionDisable  = "subscription.disable"
	EventSubscriptionExpiring = "subscription.expiring_cards"
	EventInvoiceCreate        = "invoice.create"
	EventInvoicePaymentFailed = "invoice.payment_failed"
	EventInvoiceUpdate        = "invoice.update"
)

// WebhookEvent is the top-level structure of every Paystack webhook payload.
type WebhookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ChargeData is the payload for charge.success events.
type ChargeData struct {
	ID        int64  `json:"id"`
	Reference string `json:"reference"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	Channel   string `json:"channel"`
	Customer  struct {
		Email string `json:"email"`
	} `json:"customer"`
	Metadata map[string]interface{} `json:"metadata"`
}

// TransferData is the payload for transfer.success / transfer.failed events.
type TransferData struct {
	TransferCode  string `json:"transfer_code"`
	Reference     string `json:"reference"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
	Recipient     struct {
		RecipientCode string `json:"recipient_code"`
	} `json:"recipient"`
}

// SubscriptionData is the payload for subscription.* events.
type SubscriptionData struct {
	SubscriptionCode string `json:"subscription_code"`
	EmailToken       string `json:"email_token"`
	Status           string `json:"status"`
	Amount           int64  `json:"amount"`
	NextPaymentDate  string `json:"next_payment_date"`
	Plan             struct {
		PlanCode string `json:"plan_code"`
		Name     string `json:"name"`
	} `json:"plan"`
	Customer struct {
		Email string `json:"email"`
	} `json:"customer"`
}

// VerifySignature validates that a webhook request genuinely came from Paystack.
// Paystack signs every webhook with HMAC-SHA512 using your secret key.
// Always call this before processing any webhook payload.
func VerifySignature(r *http.Request, secretKey string) ([]byte, error) {
	signature := r.Header.Get("X-Paystack-Signature")
	if signature == "" {
		return nil, errors.New("paystack webhook: missing X-Paystack-Signature header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("paystack webhook: failed to read request body")
	}

	mac := hmac.New(sha512.New, []byte(secretKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, errors.New("paystack webhook: signature mismatch — possible replay attack")
	}

	return body, nil
}

// ParseEvent decodes a raw webhook body into a WebhookEvent.
func ParseEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, errors.New("paystack webhook: failed to parse event body")
	}
	return &event, nil
}

// ParseChargeData decodes the Data field of a charge.success event.
func ParseChargeData(raw json.RawMessage) (*ChargeData, error) {
	var data ChargeData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// ParseTransferData decodes the Data field of a transfer.* event.
func ParseTransferData(raw json.RawMessage) (*TransferData, error) {
	var data TransferData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// ParseSubscriptionData decodes the Data field of a subscription.* event.
func ParseSubscriptionData(raw json.RawMessage) (*SubscriptionData, error) {
	var data SubscriptionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
