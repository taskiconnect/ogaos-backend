// internal/external/imagekit/client.go
package imagekit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	uploadURL = "https://upload.imagekit.io/api/v1/files/upload"
	baseURL   = "https://ik.imagekit.io"
)

type Client struct {
	publicKey   string
	privateKey  string
	urlEndpoint string // e.g. https://ik.imagekit.io/your_id
	httpClient  *http.Client
}

func NewClient(publicKey, privateKey, urlEndpoint string) *Client {
	return &Client{
		publicKey:   publicKey,
		privateKey:  privateKey,
		urlEndpoint: strings.TrimRight(urlEndpoint, "/"),
		httpClient:  &http.Client{Timeout: 60 * time.Second},
	}
}

// ─── Upload ──────────────────────────────────────────────────────────────────

type UploadRequest struct {
	File      []byte
	FileName  string
	Folder    string // e.g. "logos", "products", "receipts", "digital-products"
	IsPrivate bool   // true for downloadable files — requires signed URL to access
}

type UploadResponse struct {
	FileID   string `json:"fileId"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Size     int    `json:"size"`
	FilePath string `json:"filePath"`
}

// Upload sends a file to ImageKit and returns its URL and metadata.
// Set IsPrivate=true for digital product files (requires signed URL to access).
// Set IsPrivate=false for logos, product images, cover images (publicly accessible).
func (c *Client) Upload(req UploadRequest) (*UploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// File field
	part, err := writer.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("imagekit: create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(req.File)); err != nil {
		return nil, fmt.Errorf("imagekit: write file data: %w", err)
	}

	_ = writer.WriteField("fileName", req.FileName)
	if req.Folder != "" {
		_ = writer.WriteField("folder", req.Folder)
	}
	if req.IsPrivate {
		_ = writer.WriteField("isPrivateFile", "true")
	}

	writer.Close()

	httpReq, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return nil, fmt.Errorf("imagekit: build upload request: %w", err)
	}

	// ImageKit uses HTTP Basic auth: private_key as username, empty password
	auth := base64.StdEncoding.EncodeToString([]byte(c.privateKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+auth)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("imagekit: upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("imagekit: upload returned %d: %s", resp.StatusCode, string(b))
	}

	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("imagekit: decode upload response: %w", err)
	}
	return &result, nil
}

// ─── Signed URLs (for private files) ─────────────────────────────────────────

// SignedURLOptions configures how long a signed URL is valid for.
type SignedURLOptions struct {
	FilePath string        // ImageKit file path e.g. /digital-products/guide.pdf
	Expiry   time.Duration // How long the URL should be valid
}

// GetSignedURL generates a time-limited signed URL for a private file.
// Use this to grant a buyer access to a digital product download.
// The URL expires after the specified duration — enforce re-generation in the service layer.
func (c *Client) GetSignedURL(opts SignedURLOptions) string {
	expireAt := strconv.FormatInt(time.Now().Add(opts.Expiry).Unix(), 10)
	path := strings.TrimLeft(opts.FilePath, "/")
	urlToSign := c.urlEndpoint + "/" + path

	// HMAC-SHA1 signature
	mac := hmac.New(sha1.New, []byte(c.privateKey))
	mac.Write([]byte(urlToSign + expireAt))
	signature := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s?ik-s=%s&ik-t=%s", urlToSign, signature, expireAt)
}

// DeleteFile removes a file from ImageKit by its fileId.
// Call this when a digital product is deleted by the owner.
func (c *Client) DeleteFile(fileID string) error {
	url := fmt.Sprintf("https://api.imagekit.io/v1/files/%s", fileID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("imagekit: build delete request: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.privateKey + ":"))
	req.Header.Set("Authorization", "Basic "+auth)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("imagekit: delete request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("imagekit: delete returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
