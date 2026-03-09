// internal/external/flutterwave/client.go
package flutterwave

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.flutterwave.com/v3"

type Client struct {
	secretKey  string
	httpClient *http.Client
}

func NewClient(secretKey string) *Client {
	return &Client{
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ─── Payment Initiation ───────────────────────────────────────────────────────

type InitiatePaymentRequest struct {
	TxRef          string                 `json:"tx_ref"`
	Amount         float64                `json:"amount"`
	Currency       string                 `json:"currency"`
	RedirectURL    string                 `json:"redirect_url"`
	Customer       CustomerInfo           `json:"customer"`
	Customizations Customization          `json:"customizations"`
	Meta           map[string]interface{} `json:"meta,omitempty"`
}

type CustomerInfo struct {
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Name        string `json:"name"`
}

type Customization struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Logo        string `json:"logo,omitempty"`
}

type InitiatePaymentResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Link string `json:"link"` // redirect URL to Flutterwave checkout
	} `json:"data"`
}

// InitiatePayment creates a Flutterwave payment link.
// Used for digital store purchases as an alternative to Paystack.
func (c *Client) InitiatePayment(req InitiatePaymentRequest) (*InitiatePaymentResponse, error) {
	resp, err := c.post("/payments", req)
	if err != nil {
		return nil, fmt.Errorf("flutterwave: initiate payment: %w", err)
	}
	defer resp.Body.Close()
	var result InitiatePaymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("flutterwave: decode initiate response: %w", err)
	}
	return &result, nil
}

// ─── Transaction Verification ─────────────────────────────────────────────────

type VerifyTransactionResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		ID            int64   `json:"id"`
		TxRef         string  `json:"tx_ref"`
		FlwRef        string  `json:"flw_ref"`
		Amount        float64 `json:"amount"`
		ChargedAmount float64 `json:"charged_amount"`
		Currency      string  `json:"currency"`
		Status        string  `json:"status"` // successful | failed | pending
		PaymentType   string  `json:"payment_type"`
		Customer      struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"customer"`
		Meta map[string]interface{} `json:"meta"`
	} `json:"data"`
}

// VerifyTransaction confirms a Flutterwave payment by transaction ID.
func (c *Client) VerifyTransaction(transactionID string) (*VerifyTransactionResponse, error) {
	url := fmt.Sprintf("%s/transactions/%s/verify", baseURL, transactionID)
	resp, err := c.get(url)
	if err != nil {
		return nil, fmt.Errorf("flutterwave: verify transaction: %w", err)
	}
	defer resp.Body.Close()
	var result VerifyTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("flutterwave: decode verify response: %w", err)
	}
	return &result, nil
}

func (c *Client) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

func (c *Client) post(path string, body interface{}) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flutterwave: POST %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("flutterwave: POST %s returned %d: %s", path, resp.StatusCode, string(b))
	}
	return resp, nil
}
