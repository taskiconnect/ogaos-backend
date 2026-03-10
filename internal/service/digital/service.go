// internal/service/digital/service.go
package digital

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/external/imagekit"
	"ogaos-backend/internal/pkg/cursor"
	"ogaos-backend/internal/pkg/email"
)

type Service struct {
	db                 *gorm.DB
	imagekitClient     *imagekit.Client
	frontendURL        string
	platformFeePercent int
}

func NewService(db *gorm.DB, ikClient *imagekit.Client, frontendURL string, platformFeePercent int) *Service {
	return &Service{
		db:                 db,
		imagekitClient:     ikClient,
		frontendURL:        frontendURL,
		platformFeePercent: platformFeePercent,
	}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type CreateRequest struct {
	Title         string  `json:"title" binding:"required"`
	Description   string  `json:"description" binding:"required"`
	Type          string  `json:"type" binding:"required"`
	Price         int64   `json:"price" binding:"required,min=1"`
	PromoVideoURL *string `json:"promo_video_url"`
}

type UpdateRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Price         *int64  `json:"price"`
	PromoVideoURL *string `json:"promo_video_url"`
	IsPublished   *bool   `json:"is_published"`
}

type PurchaseRequest struct {
	BuyerName  string `json:"buyer_name" binding:"required"`
	BuyerEmail string `json:"buyer_email" binding:"required,email"`
	Reference  string `json:"reference" binding:"required"` // Paystack/Flutterwave transaction ref
	Channel    string `json:"channel" binding:"required"`   // "paystack" | "flutterwave"
}

// ─── Product management ───────────────────────────────────────────────────────

func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.DigitalProduct, error) {
	p := models.DigitalProduct{
		BusinessID:    businessID,
		Title:         req.Title,
		Slug:          s.generateSlug(req.Title),
		Description:   req.Description,
		Type:          req.Type,
		Price:         req.Price,
		PromoVideoURL: req.PromoVideoURL,
	}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) Get(businessID, productID uuid.UUID) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("digital product not found")
	}
	return &p, nil
}

// GetPublic returns a published digital product by slug for the storefront.
func (s *Service) GetPublic(slug string) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("slug = ? AND is_published = true", slug).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}
	return &p, nil
}

func (s *Service) List(businessID uuid.UUID, cur string, limit int) ([]models.DigitalProduct, string, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}

	q := s.db.Model(&models.DigitalProduct{}).Where("business_id = ?", businessID)

	if cur != "" {
		cursorTime, id, err := cursor.Decode(cur)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cursorTime, id)
	}

	var products []models.DigitalProduct
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&products).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(products) > limit {
		last := products[limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		products = products[:limit]
	}
	return products, nextCursor, nil
}

func (s *Service) Update(businessID, productID uuid.UUID, req UpdateRequest) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("digital product not found")
	}
	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.PromoVideoURL != nil {
		updates["promo_video_url"] = *req.PromoVideoURL
	}
	if req.IsPublished != nil {
		if *req.IsPublished && p.FileURL == nil {
			return nil, errors.New("cannot publish a product without uploading a file first")
		}
		updates["is_published"] = *req.IsPublished
	}
	if err := s.db.Model(&p).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) Delete(businessID, productID uuid.UUID) error {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return errors.New("digital product not found")
	}
	// Clean up ImageKit files if stored
	if p.FileURL != nil {
		// FileID is stored separately — handled by upload service
	}
	return s.db.Delete(&p).Error
}

// AttachFile sets the private file URL after upload service stores it in ImageKit.
func (s *Service) AttachFile(businessID, productID uuid.UUID, fileURL string, fileSize int64, mimeType string) error {
	result := s.db.Model(&models.DigitalProduct{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Updates(map[string]interface{}{
			"file_url":       fileURL,
			"file_size":      fileSize,
			"file_mime_type": mimeType,
		})
	if result.RowsAffected == 0 {
		return errors.New("digital product not found")
	}
	return result.Error
}

// AttachCoverImage sets the cover image URL.
func (s *Service) AttachCoverImage(businessID, productID uuid.UUID, coverURL string) error {
	result := s.db.Model(&models.DigitalProduct{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Update("cover_image_url", coverURL)
	if result.RowsAffected == 0 {
		return errors.New("digital product not found")
	}
	return result.Error
}

// ─── Purchase ─────────────────────────────────────────────────────────────────

// CompletePurchase is called after Paystack/Flutterwave confirms payment.
// Creates a DigitalOrder, sends the signed download link to the buyer.
func (s *Service) CompletePurchase(productID uuid.UUID, req PurchaseRequest) (*models.DigitalOrder, error) {
	var p models.DigitalProduct
	if err := s.db.Preload("Business").First(&p, productID).Error; err != nil {
		return nil, errors.New("product not found")
	}
	if !p.IsPublished {
		return nil, errors.New("product is not available for purchase")
	}

	accessToken := uuid.NewString()
	now := time.Now()
	accessExpiry := now.Add(48 * time.Hour)

	order := models.DigitalOrder{
		DigitalProductID: productID,
		BusinessID:       p.BusinessID,
		BuyerName:        req.BuyerName,
		BuyerEmail:       req.BuyerEmail,
		AmountPaid:       p.Price,
		Currency:         p.Currency,
		PaymentChannel:   req.Channel, // "paystack" | "flutterwave"
		PaymentReference: &req.Reference,
		PaymentStatus:    models.OrderPaymentStatusSuccessful,
		PaidAt:           &now,
		AccessGranted:    true,
		AccessToken:      &accessToken,
		AccessExpiresAt:  &accessExpiry,
		PayoutStatus:     models.PayoutStatusPending,
	}
	order.CalculateFees() // sets PlatformFee and OwnerPayoutAmount from AmountPaid

	// Build signed download URL to email (not stored — generated fresh each time)
	var downloadURL string
	if p.FileURL != nil {
		downloadURL = s.imagekitClient.GetSignedURL(imagekit.SignedURLOptions{
			FilePath: *p.FileURL,
			Expiry:   48 * time.Hour,
		})
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		return tx.Model(&models.DigitalProduct{}).Where("id = ?", productID).Updates(map[string]interface{}{
			"sales_count":   gorm.Expr("sales_count + 1"),
			"total_revenue": gorm.Expr("total_revenue + ?", order.OwnerPayoutAmount),
		}).Error
	}); err != nil {
		return nil, err
	}

	// Email the signed download link to the buyer
	if downloadURL != "" {
		email.SendDigitalProductAccess(
			req.BuyerEmail,
			req.BuyerName,
			p.Title,
			p.Business.Name,
			downloadURL,
		)
	}

	return &order, nil
}

// GetDownloadURL returns a fresh signed URL for an existing order.
// Called when the buyer requests a re-download.
func (s *Service) GetDownloadURL(orderID uuid.UUID, buyerEmail string) (string, error) {
	var order models.DigitalOrder
	if err := s.db.Where("id = ? AND buyer_email = ?", orderID, buyerEmail).
		Preload("DigitalProduct").First(&order).Error; err != nil {
		return "", errors.New("order not found")
	}
	if order.DigitalProduct.FileURL == nil {
		return "", errors.New("file not available")
	}
	return s.imagekitClient.GetSignedURL(imagekit.SignedURLOptions{
		FilePath: *order.DigitalProduct.FileURL,
		Expiry:   24 * time.Hour,
	}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) generateSlug(title string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, title)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return fmt.Sprintf("%s-%d", slug, time.Now().UnixMilli())
}
