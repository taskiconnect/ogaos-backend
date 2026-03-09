// internal/external/paystack/transfer.go
package paystack

import (
	"encoding/json"
	"fmt"
)

// ─── Transfer (payout to business owner) ─────────────────────────────────────

type InitiateTransferRequest struct {
	Source    string `json:"source"`    // always "balance"
	Amount    int64  `json:"amount"`    // in kobo
	Recipient string `json:"recipient"` // recipient_code from CreateRecipient
	Reason    string `json:"reason"`
	Reference string `json:"reference"` // our idempotency key — order_id
}

type InitiateTransferResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		TransferCode string `json:"transfer_code"`
		Reference    string `json:"reference"`
		Status       string `json:"status"` // pending | success | failed
		Amount       int64  `json:"amount"`
	} `json:"data"`
}

// InitiateTransfer sends funds to a business owner's bank account.
// The transfer is asynchronous — the final status arrives via transfer.success /
// transfer.failed webhook events.
func (c *Client) InitiateTransfer(req InitiateTransferRequest) (*InitiateTransferResponse, error) {
	resp, err := c.post("/transfer", req)
	if err != nil {
		return nil, fmt.Errorf("paystack: initiate transfer: %w", err)
	}
	defer resp.Body.Close()
	var result InitiateTransferResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode transfer response: %w", err)
	}
	if !result.Status {
		return nil, fmt.Errorf("paystack: transfer failed: %s", result.Message)
	}
	return &result, nil
}
