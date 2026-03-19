// internal/service/digital/service.go
package digital

import (
	"encoding/json"
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

// ─── DTOs ─────────────────────────────────────────────────────────────────────

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
	Reference  string `json:"reference" binding:"required"` // Paystack/Flutterwave ref
	Channel    string `json:"channel" binding:"required"`   // "paystack" | "flutterwave"
}

// ─── Product management ───────────────────────────────────────────────────────

// Create creates a new digital product and sets its 180-day expiry clock.
func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.DigitalProduct, error) {
	expiry := time.Now().AddDate(0, 0, models.DigitalProductLifetimeDays)
	p := models.DigitalProduct{
		BusinessID:       businessID,
		Title:            req.Title,
		Slug:             s.generateSlug(req.Title),
		Description:      req.Description,
		Type:             req.Type,
		Price:            req.Price,
		PromoVideoURL:    req.PromoVideoURL,
		GalleryImageURLs: "[]",
		ExpiresAt:        &expiry,
	}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// Get returns a digital product owned by the given business.
func (s *Service) Get(businessID, productID uuid.UUID) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("digital product not found")
	}
	return &p, nil
}

// GetPublic returns a published digital product by slug for the public storefront.
func (s *Service) GetPublic(slug string) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("slug = ? AND is_published = true", slug).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}
	return &p, nil
}

// ListPublic returns all published products for a business identified by its slug.
// Used by the public storefront — no authentication required.
func (s *Service) ListPublic(businessSlug string) ([]models.DigitalProduct, error) {
	var b models.Business
	if err := s.db.Select("id").Where("slug = ? AND is_profile_public = true", businessSlug).First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}
	var products []models.DigitalProduct
	err := s.db.
		Where("business_id = ? AND is_published = true", b.ID).
		Order("created_at DESC").
		Find(&products).Error
	return products, err
}

// List returns digital products for the given business using cursor-based pagination.
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

// Update updates editable fields on a digital product.
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

// Delete removes a digital product.
func (s *Service) Delete(businessID, productID uuid.UUID) error {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return errors.New("digital product not found")
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

// ─── Gallery ──────────────────────────────────────────────────────────────────

// AddGalleryImage adds an image URL to the product's gallery (max 3).
func (s *Service) AddGalleryImage(businessID, productID uuid.UUID, imageURL string) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("digital product not found")
	}
	gallery := parseGallery(p.GalleryImageURLs)
	if len(gallery) >= 3 {
		return nil, errors.New("maximum 3 gallery images allowed per product")
	}
	gallery = append(gallery, imageURL)
	if err := s.db.Model(&p).Update("gallery_image_urls", marshalGallery(gallery)).Error; err != nil {
		return nil, err
	}
	p.GalleryImageURLs = marshalGallery(gallery)
	return &p, nil
}

// RemoveGalleryImage removes a gallery image at the given zero-based index.
func (s *Service) RemoveGalleryImage(businessID, productID uuid.UUID, index int) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("digital product not found")
	}
	gallery := parseGallery(p.GalleryImageURLs)
	if index < 0 || index >= len(gallery) {
		return nil, errors.New("invalid gallery index")
	}
	gallery = append(gallery[:index], gallery[index+1:]...)
	if err := s.db.Model(&p).Update("gallery_image_urls", marshalGallery(gallery)).Error; err != nil {
		return nil, err
	}
	p.GalleryImageURLs = marshalGallery(gallery)
	return &p, nil
}

// ─── Expiry ───────────────────────────────────────────────────────────────────

// ExpireOldProducts unpublishes digital products that have passed their expires_at date.
// Also catches products without an explicit expires_at that are older than 180 days.
// Call this from your daily scheduler.
func (s *Service) ExpireOldProducts() (int64, error) {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -models.DigitalProductLifetimeDays)

	// Expire products with explicit expires_at that has passed
	r1 := s.db.Model(&models.DigitalProduct{}).
		Where("is_published = true AND expires_at IS NOT NULL AND expires_at < ?", now).
		Update("is_published", false)
	if r1.Error != nil {
		return 0, r1.Error
	}

	// Also catch legacy products without expires_at that are older than 180 days
	r2 := s.db.Model(&models.DigitalProduct{}).
		Where("is_published = true AND expires_at IS NULL AND created_at < ?", cutoff).
		Updates(map[string]interface{}{
			"is_published": false,
			"expires_at":   cutoff,
		})

	return r1.RowsAffected + r2.RowsAffected, r2.Error
}

// ─── Purchase ─────────────────────────────────────────────────────────────────

// CompletePurchase is called after Paystack/Flutterwave confirms payment via webhook.
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
		PaymentChannel:   req.Channel,
		PaymentReference: &req.Reference,
		PaymentStatus:    models.OrderPaymentStatusSuccessful,
		PaidAt:           &now,
		AccessGranted:    true,
		AccessToken:      &accessToken,
		AccessExpiresAt:  &accessExpiry,
		PayoutStatus:     models.PayoutStatusPending,
	}
	order.CalculateFees()

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

func parseGallery(raw string) []string {
	if raw == "" || raw == "null" {
		return []string{}
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return []string{}
	}
	return urls
}

func marshalGallery(urls []string) string {
	if urls == nil {
		urls = []string{}
	}
	b, _ := json.Marshal(urls)
	return string(b)
}

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
