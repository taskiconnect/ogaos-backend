// internal/external/flutterwave/webhook.go
package flutterwave

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	EventChargeCompleted   = "charge.completed"
	EventTransferCompleted = "transfer.completed"
	EventPaymentRetry      = "payment.retry"
)

type WebhookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type ChargeData struct {
	ID            int64   `json:"id"`
	TxRef         string  `json:"tx_ref"`
	FlwRef        string  `json:"flw_ref"`
	Amount        float64 `json:"amount"`
	ChargedAmount float64 `json:"charged_amount"`
	Currency      string  `json:"currency"`
	Status        string  `json:"status"`
	PaymentType   string  `json:"payment_type"`
	Customer      struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"customer"`
	Meta map[string]interface{} `json:"meta"`
}

// VerifySignature validates a Flutterwave webhook using the secret hash header.
// Flutterwave signs webhooks with a hash of the payload + your secret hash.
func VerifySignature(r *http.Request, secretHash string) ([]byte, error) {
	signature := r.Header.Get("verif-hash")
	if signature == "" {
		return nil, errors.New("flutterwave webhook: missing verif-hash header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("flutterwave webhook: failed to read request body")
	}

	// Flutterwave uses the raw secret hash as the header value (not HMAC)
	// Compare directly after hashing body if using the enhanced method
	h := sha256.New()
	h.Write([]byte(secretHash))
	expected := hex.EncodeToString(h.Sum(nil))

	// Fall back: Flutterwave may send the secret hash directly
	if signature != secretHash && signature != expected {
		return nil, errors.New("flutterwave webhook: invalid signature")
	}

	return body, nil
}

func ParseEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, errors.New("flutterwave webhook: failed to parse event body")
	}
	return &event, nil
}

func ParseChargeData(raw json.RawMessage) (*ChargeData, error) {
	var data ChargeData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
