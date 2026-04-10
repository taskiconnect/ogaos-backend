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
		frontendURL:        strings.TrimRight(frontendURL, "/"),
		platformFeePercent: platformFeePercent,
	}
}

type CreateRequest struct {
	Title               string  `json:"title" binding:"required"`
	Description         string  `json:"description" binding:"required"`
	Type                string  `json:"type" binding:"required"`
	Price               int64   `json:"price" binding:"required,min=1"`
	PromoVideoURL       *string `json:"promo_video_url"`
	FulfillmentMode     *string `json:"fulfillment_mode"`
	AccessRedirectURL   *string `json:"access_redirect_url"`
	RequiresAccount     *bool   `json:"requires_account"`
	AccessDurationHours *int    `json:"access_duration_hours"`
	DeliveryNote        *string `json:"delivery_note"`
}

type UpdateRequest struct {
	Title               *string `json:"title"`
	Description         *string `json:"description"`
	Price               *int64  `json:"price"`
	PromoVideoURL       *string `json:"promo_video_url"`
	IsPublished         *bool   `json:"is_published"`
	FulfillmentMode     *string `json:"fulfillment_mode"`
	AccessRedirectURL   *string `json:"access_redirect_url"`
	RequiresAccount     *bool   `json:"requires_account"`
	AccessDurationHours *int    `json:"access_duration_hours"`
	DeliveryNote        *string `json:"delivery_note"`
}

type PurchaseRequest struct {
	BuyerName  string `json:"buyer_name" binding:"required"`
	BuyerEmail string `json:"buyer_email" binding:"required,email"`
	Reference  string `json:"reference" binding:"required"`
	Channel    string `json:"channel" binding:"required"`
}

type FulfillmentResponse struct {
	OrderID           uuid.UUID  `json:"order_id"`
	ProductID         uuid.UUID  `json:"product_id"`
	ProductTitle      string     `json:"product_title"`
	ProductType       string     `json:"product_type"`
	FulfillmentMode   string     `json:"fulfillment_mode"`
	FulfillmentStatus string     `json:"fulfillment_status"`
	PaymentStatus     string     `json:"payment_status"`
	AccessGranted     bool       `json:"access_granted"`
	RequiresAccount   bool       `json:"requires_account"`
	RedirectURL       *string    `json:"redirect_url,omitempty"`
	DownloadToken     *string    `json:"download_token,omitempty"`
	DeliveryNote      *string    `json:"delivery_note,omitempty"`
	AccessExpiresAt   *time.Time `json:"access_expires_at,omitempty"`
	Message           string     `json:"message"`
}

type PurchaseResult struct {
	Message       string              `json:"message"`
	OrderID       uuid.UUID           `json:"order_id"`
	AccessToken   *string             `json:"access_token,omitempty"`
	CompletionURL string              `json:"completion_url"`
	Fulfillment   FulfillmentResponse `json:"fulfillment"`
}

func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.DigitalProduct, error) {
	mode := s.defaultFulfillmentMode(req.Type, req.FulfillmentMode)
	if err := s.validateFulfillmentMode(mode); err != nil {
		return nil, err
	}

	requiresAccount := false
	if req.RequiresAccount != nil {
		requiresAccount = *req.RequiresAccount
	}

	p := models.DigitalProduct{
		BusinessID:          businessID,
		Title:               req.Title,
		Slug:                s.generateSlug(req.Title),
		Description:         req.Description,
		Type:                req.Type,
		Price:               req.Price,
		PromoVideoURL:       req.PromoVideoURL,
		FulfillmentMode:     mode,
		AccessRedirectURL:   req.AccessRedirectURL,
		RequiresAccount:     requiresAccount,
		AccessDurationHours: req.AccessDurationHours,
		DeliveryNote:        req.DeliveryNote,
		GalleryImageURLs:    "[]",
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

func (s *Service) GetPublic(slug string) (*models.DigitalProduct, error) {
	var p models.DigitalProduct
	if err := s.db.Where("slug = ? AND is_published = true", slug).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}
	return &p, nil
}

func (s *Service) ListPublic(businessSlug string) ([]models.DigitalProduct, error) {
	var b models.Business
	if err := s.db.Select("id").
		Where("slug = ? AND is_profile_public = true", businessSlug).
		First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}

	var products []models.DigitalProduct
	err := s.db.
		Where("business_id = ? AND is_published = true", b.ID).
		Order("created_at DESC").
		Find(&products).Error

	return products, err
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
	if req.FulfillmentMode != nil {
		if err := s.validateFulfillmentMode(*req.FulfillmentMode); err != nil {
			return nil, err
		}
		updates["fulfillment_mode"] = *req.FulfillmentMode
		p.FulfillmentMode = *req.FulfillmentMode
	}
	if req.AccessRedirectURL != nil {
		updates["access_redirect_url"] = *req.AccessRedirectURL
		p.AccessRedirectURL = req.AccessRedirectURL
	}
	if req.RequiresAccount != nil {
		updates["requires_account"] = *req.RequiresAccount
		p.RequiresAccount = *req.RequiresAccount
	}
	if req.AccessDurationHours != nil {
		updates["access_duration_hours"] = *req.AccessDurationHours
		p.AccessDurationHours = req.AccessDurationHours
	}
	if req.DeliveryNote != nil {
		updates["delivery_note"] = *req.DeliveryNote
		p.DeliveryNote = req.DeliveryNote
	}
	if req.IsPublished != nil {
		if *req.IsPublished {
			if err := s.validateProductForPublishing(&p); err != nil {
				return nil, err
			}
		}
		updates["is_published"] = *req.IsPublished
	}

	if err := s.db.Model(&models.DigitalProduct{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.Get(businessID, productID)
}

func (s *Service) Delete(businessID, productID uuid.UUID) error {
	var p models.DigitalProduct
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return errors.New("digital product not found")
	}
	return s.db.Delete(&p).Error
}

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

func (s *Service) AttachCoverImage(businessID, productID uuid.UUID, coverURL string) error {
	result := s.db.Model(&models.DigitalProduct{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Update("cover_image_url", coverURL)
	if result.RowsAffected == 0 {
		return errors.New("digital product not found")
	}
	return result.Error
}

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

// kept for backward compatibility with any scheduler that may still call it
func (s *Service) ExpireOldProducts() (int64, error) {
	return 0, nil
}

func (s *Service) ListOrders(businessID uuid.UUID, cur string, limit int) ([]models.DigitalOrder, string, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}

	q := s.db.Model(&models.DigitalOrder{}).
		Where("business_id = ?", businessID).
		Preload("DigitalProduct")

	if cur != "" {
		cursorTime, id, err := cursor.Decode(cur)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cursorTime, id)
	}

	var orders []models.DigitalOrder
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&orders).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(orders) > limit {
		last := orders[limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		orders = orders[:limit]
	}

	return orders, nextCursor, nil
}

func (s *Service) GetOrder(businessID, orderID uuid.UUID) (*models.DigitalOrder, error) {
	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND business_id = ?", orderID, businessID).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return nil, errors.New("order not found")
	}
	return &order, nil
}

func (s *Service) ResendAccessLink(businessID, orderID uuid.UUID) (string, error) {
	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND business_id = ?", orderID, businessID).
		Preload("DigitalProduct").
		Preload("Business").
		First(&order).Error; err != nil {
		return "", errors.New("order not found")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return "", errors.New("payment is not successful for this order")
	}

	if order.AccessToken == nil || strings.TrimSpace(*order.AccessToken) == "" {
		token := uuid.NewString()
		order.AccessToken = &token
		if err := s.db.Model(&order).Update("access_token", token).Error; err != nil {
			return "", err
		}
	}

	completionURL := s.buildCompletionURL(order.ID, *order.AccessToken)

	email.SendDigitalProductAccess(
		order.BuyerEmail,
		order.BuyerName,
		order.DigitalProduct.Title,
		order.Business.Name,
		completionURL,
	)

	return completionURL, nil
}

func (s *Service) CompletePurchase(productID uuid.UUID, req PurchaseRequest) (*PurchaseResult, error) {
	// idempotency by payment reference
	if req.Reference != "" {
		var existing models.DigitalOrder
		err := s.db.
			Where("payment_reference = ?", req.Reference).
			Preload("DigitalProduct").
			First(&existing).Error

		if err == nil {
			return s.buildPurchaseResult(&existing), nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	var p models.DigitalProduct
	if err := s.db.Preload("Business").First(&p, productID).Error; err != nil {
		return nil, errors.New("product not found")
	}
	if !p.IsPublished {
		return nil, errors.New("product is not available for purchase")
	}
	if err := s.validateProductForPublishing(&p); err != nil {
		return nil, err
	}

	status, granted, accessURL, err := s.resolveFulfillment(&p)
	if err != nil {
		return nil, err
	}

	accessToken := uuid.NewString()
	now := time.Now()

	var accessExpiry *time.Time
	if p.AccessDurationHours != nil && *p.AccessDurationHours > 0 {
		t := now.Add(time.Duration(*p.AccessDurationHours) * time.Hour)
		accessExpiry = &t
	}

	order := models.DigitalOrder{
		DigitalProductID:  productID,
		BusinessID:        p.BusinessID,
		BuyerName:         req.BuyerName,
		BuyerEmail:        req.BuyerEmail,
		AmountPaid:        p.Price,
		Currency:          p.Currency,
		PaymentChannel:    req.Channel,
		PaymentReference:  &req.Reference,
		PaymentStatus:     models.OrderPaymentStatusSuccessful,
		PaidAt:            &now,
		FulfillmentMode:   p.FulfillmentMode,
		FulfillmentStatus: status,
		AccessGranted:     granted,
		AccessToken:       &accessToken,
		AccessExpiresAt:   accessExpiry,
		AccessURL:         accessURL,
		PayoutStatus:      models.PayoutStatusPending,
	}
	order.CalculateFees()

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		return tx.Model(&models.DigitalProduct{}).
			Where("id = ?", productID).
			Updates(map[string]interface{}{
				"sales_count":   gorm.Expr("sales_count + 1"),
				"total_revenue": gorm.Expr("total_revenue + ?", order.OwnerPayoutAmount),
			}).Error
	}); err != nil {
		return nil, err
	}

	completionURL := s.buildCompletionURL(order.ID, accessToken)

	email.SendDigitalProductAccess(
		req.BuyerEmail,
		req.BuyerName,
		p.Title,
		p.Business.Name,
		completionURL,
	)

	order.DigitalProduct = p
	return &PurchaseResult{
		Message:       "Purchase completed successfully",
		OrderID:       order.ID,
		AccessToken:   order.AccessToken,
		CompletionURL: completionURL,
		Fulfillment:   s.buildFulfillmentResponse(&order),
	}, nil
}

func (s *Service) GetFulfillment(orderID uuid.UUID, token string) (*FulfillmentResponse, error) {
	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND access_token = ?", orderID, token).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return nil, errors.New("invalid order access")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return nil, errors.New("payment not completed")
	}

	if order.AccessExpiresAt != nil && time.Now().After(*order.AccessExpiresAt) {
		return nil, errors.New("access has expired")
	}

	res := s.buildFulfillmentResponse(&order)
	return &res, nil
}

func (s *Service) GetDownloadURL(orderID uuid.UUID, buyerEmail string) (string, error) {
	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND buyer_email = ?", orderID, buyerEmail).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
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

func (s *Service) GetDownloadURLByToken(token string) (string, error) {
	var order models.DigitalOrder
	if err := s.db.
		Where("access_token = ?", token).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return "", errors.New("download access not found")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return "", errors.New("payment not completed")
	}
	if order.FulfillmentMode != models.DigitalFulfillmentModeFileDownload {
		return "", errors.New("this product is not a file download")
	}
	if !order.AccessGranted || order.FulfillmentStatus != models.DigitalFulfillmentStatusReady {
		return "", errors.New("download access is not ready")
	}
	if order.AccessExpiresAt != nil && time.Now().After(*order.AccessExpiresAt) {
		return "", errors.New("download access has expired")
	}
	if order.DigitalProduct.FileURL == nil {
		return "", errors.New("file not available")
	}
	if order.MaxDownloadCount != nil && order.DownloadCount >= *order.MaxDownloadCount {
		return "", errors.New("download limit reached")
	}

	if err := s.db.Model(&models.DigitalOrder{}).
		Where("id = ?", order.ID).
		Update("download_count", gorm.Expr("download_count + 1")).Error; err != nil {
		return "", err
	}

	return s.imagekitClient.GetSignedURL(imagekit.SignedURLOptions{
		FilePath: *order.DigitalProduct.FileURL,
		Expiry:   24 * time.Hour,
	}), nil
}

func (s *Service) buildPurchaseResult(order *models.DigitalOrder) *PurchaseResult {
	var completionURL string
	if order.AccessToken != nil {
		completionURL = s.buildCompletionURL(order.ID, *order.AccessToken)
	}

	return &PurchaseResult{
		Message:       "Purchase already processed",
		OrderID:       order.ID,
		AccessToken:   order.AccessToken,
		CompletionURL: completionURL,
		Fulfillment:   s.buildFulfillmentResponse(order),
	}
}

func (s *Service) buildFulfillmentResponse(order *models.DigitalOrder) FulfillmentResponse {
	res := FulfillmentResponse{
		OrderID:           order.ID,
		ProductID:         order.DigitalProductID,
		ProductTitle:      order.DigitalProduct.Title,
		ProductType:       order.DigitalProduct.Type,
		FulfillmentMode:   order.FulfillmentMode,
		FulfillmentStatus: order.FulfillmentStatus,
		PaymentStatus:     order.PaymentStatus,
		AccessGranted:     order.AccessGranted,
		RequiresAccount:   order.DigitalProduct.RequiresAccount,
		AccessExpiresAt:   order.AccessExpiresAt,
		DeliveryNote:      order.DigitalProduct.DeliveryNote,
	}

	switch order.FulfillmentMode {
	case models.DigitalFulfillmentModeFileDownload:
		if order.AccessToken != nil && order.FulfillmentStatus == models.DigitalFulfillmentStatusReady {
			res.DownloadToken = order.AccessToken
			res.Message = "Payment confirmed. Your download is ready."
		} else {
			res.Message = "Payment confirmed. Download access is not ready yet."
		}

	case models.DigitalFulfillmentModeCourseAccess:
		if order.AccessURL != nil && order.FulfillmentStatus == models.DigitalFulfillmentStatusReady {
			res.RedirectURL = order.AccessURL
			res.Message = "Payment confirmed. Course access granted."
		} else {
			res.Message = "Payment confirmed. Course access is pending."
		}

	case models.DigitalFulfillmentModeExternalLink:
		if order.AccessURL != nil && order.FulfillmentStatus == models.DigitalFulfillmentStatusReady {
			res.RedirectURL = order.AccessURL
			res.Message = "Payment confirmed. Redirecting to your digital product."
		} else {
			res.Message = "Payment confirmed. Access link is pending."
		}

	case models.DigitalFulfillmentModeManualDelivery:
		res.Message = "Payment confirmed. The seller will deliver your digital product shortly."

	default:
		res.Message = "Payment confirmed."
	}

	return res
}

func (s *Service) resolveFulfillment(p *models.DigitalProduct) (string, bool, *string, error) {
	switch p.FulfillmentMode {
	case models.DigitalFulfillmentModeFileDownload:
		if p.FileURL == nil || strings.TrimSpace(*p.FileURL) == "" {
			return "", false, nil, errors.New("file download products must have an uploaded file")
		}
		return models.DigitalFulfillmentStatusReady, true, nil, nil

	case models.DigitalFulfillmentModeCourseAccess:
		if p.AccessRedirectURL == nil || strings.TrimSpace(*p.AccessRedirectURL) == "" {
			return "", false, nil, errors.New("course access products require an access_redirect_url")
		}
		return models.DigitalFulfillmentStatusReady, true, p.AccessRedirectURL, nil

	case models.DigitalFulfillmentModeExternalLink:
		if p.AccessRedirectURL == nil || strings.TrimSpace(*p.AccessRedirectURL) == "" {
			return "", false, nil, errors.New("external link products require an access_redirect_url")
		}
		return models.DigitalFulfillmentStatusReady, true, p.AccessRedirectURL, nil

	case models.DigitalFulfillmentModeManualDelivery:
		return models.DigitalFulfillmentStatusPending, false, nil, nil

	default:
		return "", false, nil, errors.New("invalid fulfillment mode")
	}
}

func (s *Service) validateProductForPublishing(p *models.DigitalProduct) error {
	switch p.FulfillmentMode {
	case models.DigitalFulfillmentModeFileDownload:
		if p.FileURL == nil || strings.TrimSpace(*p.FileURL) == "" {
			return errors.New("cannot publish a file download product without uploading a file first")
		}
	case models.DigitalFulfillmentModeCourseAccess:
		if p.AccessRedirectURL == nil || strings.TrimSpace(*p.AccessRedirectURL) == "" {
			return errors.New("cannot publish a course access product without an access_redirect_url")
		}
	case models.DigitalFulfillmentModeExternalLink:
		if p.AccessRedirectURL == nil || strings.TrimSpace(*p.AccessRedirectURL) == "" {
			return errors.New("cannot publish an external link product without an access_redirect_url")
		}
	case models.DigitalFulfillmentModeManualDelivery:
		// valid
	default:
		return errors.New("invalid fulfillment mode")
	}
	return nil
}

func (s *Service) defaultFulfillmentMode(productType string, explicit *string) string {
	if explicit != nil && strings.TrimSpace(*explicit) != "" {
		return strings.TrimSpace(*explicit)
	}

	switch strings.ToLower(strings.TrimSpace(productType)) {
	case models.DigitalProductTypeCourse:
		return models.DigitalFulfillmentModeCourseAccess
	case models.DigitalProductTypeVideo:
		return models.DigitalFulfillmentModeExternalLink
	case models.DigitalProductTypeService:
		return models.DigitalFulfillmentModeManualDelivery
	default:
		return models.DigitalFulfillmentModeFileDownload
	}
}

func (s *Service) validateFulfillmentMode(mode string) error {
	switch mode {
	case models.DigitalFulfillmentModeFileDownload,
		models.DigitalFulfillmentModeCourseAccess,
		models.DigitalFulfillmentModeExternalLink,
		models.DigitalFulfillmentModeManualDelivery:
		return nil
	default:
		return errors.New("invalid fulfillment mode")
	}
}

func (s *Service) buildCompletionURL(orderID uuid.UUID, token string) string {
	path := fmt.Sprintf("/public/orders/%s/complete?token=%s", orderID.String(), token)
	if s.frontendURL == "" {
		return path
	}
	return s.frontendURL + path
}

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
