// internal/external/paystack/client.go
package paystack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.paystack.co"

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

// ─── Bank / Account Resolution ──────────────────────────────────────────────

type ResolveAccountResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
		BankID        int    `json:"bank_id"`
	} `json:"data"`
}

// ResolveAccount verifies a bank account number and returns the account name.
// Used when the owner adds a payout bank account.
func (c *Client) ResolveAccount(accountNumber, bankCode string) (*ResolveAccountResponse, error) {
	url := fmt.Sprintf("%s/bank/resolve?account_number=%s&bank_code=%s", baseURL, accountNumber, bankCode)
	resp, err := c.get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result ResolveAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode resolve response: %w", err)
	}
	return &result, nil
}

type ListBanksResponse struct {
	Status bool `json:"status"`
	Data   []struct {
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"data"`
}

func (c *Client) ListBanks() (*ListBanksResponse, error) {
	url := fmt.Sprintf("%s/bank?currency=NGN&perPage=100", baseURL)
	resp, err := c.get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result ListBanksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode banks response: %w", err)
	}
	return &result, nil
}

// ─── Transfer Recipients ─────────────────────────────────────────────────────

type CreateRecipientRequest struct {
	Type          string `json:"type"` // nuban for Nigerian bank accounts
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}

type CreateRecipientResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		RecipientCode string `json:"recipient_code"`
		ID            int    `json:"id"`
	} `json:"data"`
}

// CreateRecipient creates a Paystack transfer recipient for a business payout account.
// The returned recipient_code is stored on BusinessPayoutAccount.
func (c *Client) CreateRecipient(req CreateRecipientRequest) (*CreateRecipientResponse, error) {
	resp, err := c.post("/transferrecipient", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result CreateRecipientResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode create recipient response: %w", err)
	}
	return &result, nil
}

// ─── Subscription Management ─────────────────────────────────────────────────

type InitializeTransactionRequest struct {
	Email     string                 `json:"email"`
	Amount    int64                  `json:"amount"` // in kobo
	Reference string                 `json:"reference"`
	Callback  string                 `json:"callback_url,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Plan      string                 `json:"plan,omitempty"` // Paystack plan code for recurring
}

type InitializeTransactionResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

// InitializeTransaction starts a Paystack payment session.
// Used for subscription upgrades and one-time digital product payments.
func (c *Client) InitializeTransaction(req InitializeTransactionRequest) (*InitializeTransactionResponse, error) {
	resp, err := c.post("/transaction/initialize", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result InitializeTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode initialize response: %w", err)
	}
	return &result, nil
}

type VerifyTransactionResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Status    string `json:"status"` // success | failed | abandoned
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Channel   string `json:"channel"`
		Currency  string `json:"currency"`
		Customer  struct {
			Email string `json:"email"`
		} `json:"customer"`
		Metadata map[string]interface{} `json:"metadata"`
	} `json:"data"`
}

// VerifyTransaction confirms a payment completed successfully.
// Call this after receiving a charge.success webhook or redirect.
func (c *Client) VerifyTransaction(reference string) (*VerifyTransactionResponse, error) {
	url := fmt.Sprintf("%s/transaction/verify/%s", baseURL, reference)
	resp, err := c.get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result VerifyTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("paystack: decode verify response: %w", err)
	}
	return &result, nil
}

// ─── HTTP helpers ────────────────────────────────────────────────────────────

func (c *Client) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("paystack: build GET request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

func (c *Client) post(path string, body interface{}) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("paystack: marshal request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("paystack: build POST request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack: POST %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("paystack: POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return resp, nil
}
