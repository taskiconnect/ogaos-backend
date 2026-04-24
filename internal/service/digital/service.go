package digital

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/external/imagekit"
	"ogaos-backend/internal/external/paystack"
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

type InitializeCheckoutRequest struct {
	BuyerName   string  `json:"buyer_name" binding:"required"`
	BuyerEmail  string  `json:"buyer_email" binding:"required,email"`
	BuyerPhone  *string `json:"buyer_phone"`
	CallbackURL *string `json:"callback_url"`
}

type InitializeCheckoutResponse struct {
	Message           string    `json:"message"`
	OrderID           uuid.UUID `json:"order_id"`
	Reference         string    `json:"reference"`
	AuthorizationURL  string    `json:"authorization_url"`
	AccessCode        string    `json:"access_code"`
	Amount            int64     `json:"amount"`
	Currency          string    `json:"currency"`
	PlatformFee       int64     `json:"platform_fee"`
	OwnerPayoutAmount int64     `json:"owner_payout_amount"`
}

type PublicBusinessInfo struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	LogoURL *string `json:"logo_url"`
}

type PublicProductResponse struct {
	models.DigitalProduct
	Business PublicBusinessInfo `json:"business"`
}

type PublicDigitalStoreResponse struct {
	Business PublicBusinessInfo      `json:"business"`
	Products []models.DigitalProduct `json:"products"`
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

type ownerContact struct {
	UserID    uuid.UUID
	Email     string
	FirstName string
}

func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.DigitalProduct, error) {
	mode := s.defaultFulfillmentMode(req.Type, req.FulfillmentMode)
	if err := s.validateFulfillmentMode(mode); err != nil {
		return nil, err
	}

	if req.AccessRedirectURL != nil && strings.TrimSpace(*req.AccessRedirectURL) != "" {
		if !isValidHTTPSURL(*req.AccessRedirectURL) {
			return nil, errors.New("access_redirect_url must be a valid https URL")
		}
	}

	requiresAccount := false
	if req.RequiresAccount != nil {
		requiresAccount = *req.RequiresAccount
	}

	p := models.DigitalProduct{
		BusinessID:          businessID,
		Title:               strings.TrimSpace(req.Title),
		Slug:                s.generateUniqueProductCode(),
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

func (s *Service) GetPublic(slug string) (*PublicProductResponse, error) {
	var p models.DigitalProduct
	if err := s.db.
		Model(&models.DigitalProduct{}).
		Joins("JOIN businesses ON businesses.id = digital_products.business_id").
		Where("digital_products.slug = ? AND digital_products.is_published = true AND businesses.is_profile_public = true", strings.TrimSpace(slug)).
		Preload("Business").
		First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}

	return &PublicProductResponse{
		DigitalProduct: p,
		Business: PublicBusinessInfo{
			Name:    p.Business.Name,
			Slug:    p.Business.Slug,
			LogoURL: p.Business.LogoURL,
		},
	}, nil
}

func (s *Service) ListPublic(businessSlug string) (*PublicDigitalStoreResponse, error) {
	var b models.Business
	if err := s.db.Select("id, name, slug, logo_url").
		Where("slug = ? AND is_profile_public = true", businessSlug).
		First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}

	var products []models.DigitalProduct
	if err := s.db.
		Where("business_id = ? AND is_published = true", b.ID).
		Order("created_at DESC").
		Find(&products).Error; err != nil {
		return nil, err
	}

	if products == nil {
		products = []models.DigitalProduct{}
	}

	return &PublicDigitalStoreResponse{
		Business: PublicBusinessInfo{
			Name:    b.Name,
			Slug:    b.Slug,
			LogoURL: b.LogoURL,
		},
		Products: products,
	}, nil
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
		updates["title"] = strings.TrimSpace(*req.Title)
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
		if strings.TrimSpace(*req.AccessRedirectURL) != "" && !isValidHTTPSURL(*req.AccessRedirectURL) {
			return nil, errors.New("access_redirect_url must be a valid https URL")
		}
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
		updates["delivery_note"] = req.DeliveryNote
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

	if len(updates) == 0 {
		return s.Get(businessID, productID)
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

func (s *Service) InitializePublicCheckout(productID uuid.UUID, req InitializeCheckoutRequest) (*InitializeCheckoutResponse, error) {
	if req.CallbackURL != nil && strings.TrimSpace(*req.CallbackURL) != "" && !isValidHTTPSURL(*req.CallbackURL) {
		return nil, errors.New("callback_url must be a valid https URL")
	}

	var p models.DigitalProduct
	if err := s.db.
		Model(&models.DigitalProduct{}).
		Joins("JOIN businesses ON businesses.id = digital_products.business_id").
		Where("digital_products.id = ? AND digital_products.is_published = true AND businesses.is_profile_public = true", productID).
		Preload("Business").
		First(&p).Error; err != nil {
		return nil, errors.New("product not found or not available for public checkout")
	}

	if err := s.validateProductForPublishing(&p); err != nil {
		return nil, err
	}

	reference := s.generatePaymentReference()
	orderID := uuid.New()

	order := models.DigitalOrder{
		ID:                orderID,
		DigitalProductID:  p.ID,
		BusinessID:        p.BusinessID,
		BuyerName:         strings.TrimSpace(req.BuyerName),
		BuyerEmail:        strings.ToLower(strings.TrimSpace(req.BuyerEmail)),
		BuyerPhone:        req.BuyerPhone,
		AmountPaid:        p.Price,
		Currency:          p.Currency,
		PaymentChannel:    "paystack",
		PaymentReference:  &reference,
		PaymentStatus:     models.OrderPaymentStatusPending,
		FulfillmentMode:   p.FulfillmentMode,
		FulfillmentStatus: models.DigitalFulfillmentStatusPending,
		AccessGranted:     false,
		PayoutStatus:      models.PayoutStatusPending,
	}
	order.CalculateFees()

	if err := s.db.Create(&order).Error; err != nil {
		return nil, err
	}

	client, err := s.paystackClient()
	if err != nil {
		_ = s.db.Delete(&models.DigitalOrder{}, "id = ?", order.ID).Error
		return nil, err
	}

	callbackURL := s.buildDefaultCheckoutCallback(reference)
	if req.CallbackURL != nil && strings.TrimSpace(*req.CallbackURL) != "" {
		callbackURL = strings.TrimSpace(*req.CallbackURL)
	}

	initResp, err := client.InitializeTransaction(paystack.InitializeTransactionRequest{
		Email:     order.BuyerEmail,
		Amount:    order.AmountPaid,
		Reference: reference,
		Callback:  callbackURL,
		Metadata: map[string]interface{}{
			"type":                "digital_product",
			"order_id":            order.ID.String(),
			"digital_product_id":  p.ID.String(),
			"business_id":         p.BusinessID.String(),
			"buyer_name":          order.BuyerName,
			"buyer_email":         order.BuyerEmail,
			"platform_fee":        order.PlatformFee,
			"owner_payout_amount": order.OwnerPayoutAmount,
		},
	})
	if err != nil {
		_ = s.db.Delete(&models.DigitalOrder{}, "id = ?", order.ID).Error
		return nil, err
	}

	return &InitializeCheckoutResponse{
		Message:           "payment initialized successfully",
		OrderID:           order.ID,
		Reference:         reference,
		AuthorizationURL:  initResp.Data.AuthorizationURL,
		AccessCode:        initResp.Data.AccessCode,
		Amount:            order.AmountPaid,
		Currency:          order.Currency,
		PlatformFee:       order.PlatformFee,
		OwnerPayoutAmount: order.OwnerPayoutAmount,
	}, nil
}

func (s *Service) MarkOrderPaidByReference(reference string) error {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return errors.New("reference is required")
	}

	client, err := s.paystackClient()
	if err != nil {
		return err
	}

	verifyResp, err := client.VerifyTransaction(reference)
	if err != nil {
		return err
	}
	if !verifyResp.Status || strings.ToLower(strings.TrimSpace(verifyResp.Data.Status)) != "success" {
		return errors.New("payment not successful")
	}

	var completedOrder *models.DigitalOrder
	var completionURL string

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var order models.DigitalOrder
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("payment_reference = ?", reference).
			Preload("DigitalProduct").
			Preload("Business").
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("order not found")
			}
			return err
		}

		if order.PaymentStatus == models.OrderPaymentStatusSuccessful {
			completedOrder = &order
			if order.AccessToken != nil {
				completionURL = s.buildCompletionURL(order.ID, *order.AccessToken)
			}
			return nil
		}

		if order.AmountPaid != verifyResp.Data.Amount {
			return fmt.Errorf("payment amount mismatch for order %s", order.ID.String())
		}
		if !strings.EqualFold(order.BuyerEmail, verifyResp.Data.Customer.Email) {
			return fmt.Errorf("payment email mismatch for order %s", order.ID.String())
		}
		if order.Currency != "" && verifyResp.Data.Currency != "" && !strings.EqualFold(order.Currency, verifyResp.Data.Currency) {
			return fmt.Errorf("payment currency mismatch for order %s", order.ID.String())
		}

		status, granted, accessURL, err := s.resolveFulfillment(&order.DigitalProduct)
		if err != nil {
			return err
		}

		now := time.Now()
		var accessExpiry *time.Time
		if order.DigitalProduct.AccessDurationHours != nil && *order.DigitalProduct.AccessDurationHours > 0 {
			t := now.Add(time.Duration(*order.DigitalProduct.AccessDurationHours) * time.Hour)
			accessExpiry = &t
		}

		accessToken := order.AccessToken
		if accessToken == nil || strings.TrimSpace(*accessToken) == "" {
			token := uuid.NewString()
			accessToken = &token
		}

		updates := map[string]interface{}{
			"payment_status":     models.OrderPaymentStatusSuccessful,
			"paid_at":            now,
			"payment_channel":    strings.TrimSpace(verifyResp.Data.Channel),
			"fulfillment_status": status,
			"access_granted":     granted,
			"access_token":       *accessToken,
			"access_expires_at":  accessExpiry,
			"access_url":         accessURL,
		}

		if err := tx.Model(&models.DigitalOrder{}).
			Where("id = ?", order.ID).
			Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.DigitalProduct{}).
			Where("id = ?", order.DigitalProductID).
			Updates(map[string]interface{}{
				"sales_count":   gorm.Expr("sales_count + 1"),
				"total_revenue": gorm.Expr("total_revenue + ?", order.AmountPaid),
			}).Error; err != nil {
			return err
		}

		order.PaymentStatus = models.OrderPaymentStatusSuccessful
		order.PaidAt = &now
		order.PaymentChannel = strings.TrimSpace(verifyResp.Data.Channel)
		order.FulfillmentStatus = status
		order.AccessGranted = granted
		order.AccessToken = accessToken
		order.AccessExpiresAt = accessExpiry
		order.AccessURL = accessURL

		completedOrder = &order
		completionURL = s.buildCompletionURL(order.ID, *accessToken)

		return nil
	})
	if err != nil {
		return err
	}

	if completedOrder == nil {
		return errors.New("failed to finalize order")
	}

	email.SendDigitalProductAccess(
		completedOrder.BuyerEmail,
		completedOrder.BuyerName,
		completedOrder.DigitalProduct.Title,
		completedOrder.Business.Name,
		completionURL,
	)

	_, _ = s.InitiatePayoutForOrder(completedOrder.ID)

	return nil
}

func (s *Service) InitiatePayoutForOrder(orderID uuid.UUID) (*models.DigitalOrder, error) {
	var updatedOrder *models.DigitalOrder

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var order models.DigitalOrder
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", orderID).
			Preload("DigitalProduct").
			First(&order).Error; err != nil {
			return errors.New("order not found")
		}

		if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
			return errors.New("order payment is not successful")
		}

		if order.OwnerPayoutAmount <= 0 {
			return errors.New("owner payout amount must be greater than zero")
		}

		if order.PayoutStatus == models.PayoutStatusCompleted || order.PayoutStatus == models.PayoutStatusProcessing {
			updatedOrder = &order
			return nil
		}

		var payoutAcct models.BusinessPayoutAccount
		if err := tx.
			Where("business_id = ? AND is_default = ? AND is_verified = ?", order.BusinessID, true, true).
			First(&payoutAcct).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_ = tx.Model(&models.DigitalOrder{}).
					Where("id = ?", order.ID).
					Updates(map[string]interface{}{
						"payout_status":      models.PayoutStatusPending,
						"payout_fail_reason": "verified payout account not found",
					}).Error
				return nil
			}
			return err
		}

		if payoutAcct.PaystackRecipientCode == nil || strings.TrimSpace(*payoutAcct.PaystackRecipientCode) == "" {
			_ = tx.Model(&models.DigitalOrder{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"payout_status":      models.PayoutStatusPending,
					"payout_fail_reason": "paystack recipient code not found",
				}).Error
			return nil
		}

		client, err := s.paystackClient()
		if err != nil {
			return err
		}

		payoutReference := fmt.Sprintf("payout_%s", order.ID.String())
		reason := fmt.Sprintf("Digital product payout for order %s", order.ID.String())

		transferResp, err := client.InitiateTransfer(paystack.InitiateTransferRequest{
			Source:    "balance",
			Amount:    order.OwnerPayoutAmount,
			Recipient: strings.TrimSpace(*payoutAcct.PaystackRecipientCode),
			Reason:    reason,
			Reference: payoutReference,
		})
		if err != nil {
			attempts := order.PayoutAttempts + 1
			failReason := err.Error()
			if updateErr := tx.Model(&models.DigitalOrder{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"payout_status":      models.PayoutStatusFailed,
					"payout_attempts":    attempts,
					"payout_fail_reason": failReason,
				}).Error; updateErr != nil {
				return updateErr
			}
			return nil
		}

		attempts := order.PayoutAttempts + 1
		ref := strings.TrimSpace(transferResp.Data.Reference)
		if ref == "" {
			ref = payoutReference
		}

		if err := tx.Model(&models.DigitalOrder{}).
			Where("id = ?", order.ID).
			Updates(map[string]interface{}{
				"payout_status":      models.PayoutStatusProcessing,
				"payout_reference":   ref,
				"payout_attempts":    attempts,
				"payout_fail_reason": nil,
			}).Error; err != nil {
			return err
		}

		order.PayoutStatus = models.PayoutStatusProcessing
		order.PayoutReference = &ref
		order.PayoutAttempts = attempts
		order.PayoutFailReason = nil
		updatedOrder = &order

		return nil
	})

	if err != nil {
		return nil, err
	}

	if updatedOrder == nil {
		return nil, errors.New("failed to update payout state")
	}

	return updatedOrder, nil
}

func (s *Service) MarkPayoutSuccess(reference string) error {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return errors.New("payout reference is required")
	}

	var owner ownerContact
	var order models.DigitalOrder

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("payout_reference = ?", reference).
			Preload("DigitalProduct").
			Preload("Business").
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("order not found for payout reference")
			}
			return err
		}

		if order.PayoutStatus == models.PayoutStatusCompleted {
			return nil
		}

		now := time.Now()
		if err := tx.Model(&models.DigitalOrder{}).
			Where("id = ?", order.ID).
			Updates(map[string]interface{}{
				"payout_status":       models.PayoutStatusCompleted,
				"payout_completed_at": now,
				"payout_fail_reason":  nil,
			}).Error; err != nil {
			return err
		}

		order.PayoutStatus = models.PayoutStatusCompleted
		order.PayoutCompletedAt = &now
		order.PayoutFailReason = nil

		ownerContactVal, err := s.getOwnerContactTx(tx, order.BusinessID)
		if err == nil {
			owner = *ownerContactVal
		}
		return nil
	})
	if err != nil {
		return err
	}

	if strings.TrimSpace(owner.Email) != "" {
		email.SendPayoutNotification(
			owner.Email,
			owner.FirstName,
			order.DigitalProduct.Title,
			order.OwnerPayoutAmount,
		)
	}

	return nil
}

func (s *Service) MarkPayoutFailed(reference string, reason string) error {
	reference = strings.TrimSpace(reference)
	reason = strings.TrimSpace(reason)
	if reference == "" {
		return errors.New("payout reference is required")
	}
	if reason == "" {
		reason = "transfer failed"
	}

	var owner ownerContact
	var order models.DigitalOrder

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("payout_reference = ?", reference).
			Preload("DigitalProduct").
			Preload("Business").
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("order not found for payout reference")
			}
			return err
		}

		if err := tx.Model(&models.DigitalOrder{}).
			Where("id = ?", order.ID).
			Updates(map[string]interface{}{
				"payout_status":      models.PayoutStatusFailed,
				"payout_fail_reason": reason,
			}).Error; err != nil {
			return err
		}

		order.PayoutStatus = models.PayoutStatusFailed
		order.PayoutFailReason = &reason

		ownerContactVal, err := s.getOwnerContactTx(tx, order.BusinessID)
		if err == nil {
			owner = *ownerContactVal
		}
		return nil
	})
	if err != nil {
		return err
	}

	if strings.TrimSpace(owner.Email) != "" {
		email.SendPayoutFailed(
			owner.Email,
			owner.FirstName,
			order.DigitalProduct.Title,
			order.OwnerPayoutAmount,
			reason,
		)
	}

	return nil
}

func (s *Service) GetFulfillment(orderID uuid.UUID, token string) (*FulfillmentResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("token is required")
	}

	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND access_token = ?", orderID, token).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return nil, errors.New("order not found or invalid token")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return nil, errors.New("payment has not been confirmed for this order")
	}

	if order.AccessExpiresAt != nil && time.Now().After(*order.AccessExpiresAt) {
		return nil, errors.New("access to this product has expired")
	}

	res := s.buildFulfillmentResponse(&order)
	return &res, nil
}

func (s *Service) GetDownloadURLByToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("download token is required")
	}

	var order models.DigitalOrder
	if err := s.db.
		Where("access_token = ?", token).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return "", errors.New("invalid or expired download token")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return "", errors.New("payment has not been confirmed for this order")
	}

	if order.AccessExpiresAt != nil && time.Now().After(*order.AccessExpiresAt) {
		return "", errors.New("download access has expired")
	}

	if order.DigitalProduct.FulfillmentMode != models.DigitalFulfillmentModeFileDownload {
		return "", errors.New("this order does not include a file download")
	}

	if order.DigitalProduct.FileURL == nil || strings.TrimSpace(*order.DigitalProduct.FileURL) == "" {
		return "", errors.New("file is not available for this product")
	}

	return *order.DigitalProduct.FileURL, nil
}

func (s *Service) GetDownloadURL(orderID uuid.UUID, buyerEmail string) (string, error) {
	buyerEmail = strings.ToLower(strings.TrimSpace(buyerEmail))
	if buyerEmail == "" {
		return "", errors.New("email is required")
	}

	var order models.DigitalOrder
	if err := s.db.
		Where("id = ? AND buyer_email = ?", orderID, buyerEmail).
		Preload("DigitalProduct").
		First(&order).Error; err != nil {
		return "", errors.New("order not found")
	}

	if order.PaymentStatus != models.OrderPaymentStatusSuccessful {
		return "", errors.New("payment has not been confirmed for this order")
	}

	if order.DigitalProduct.FulfillmentMode != models.DigitalFulfillmentModeFileDownload {
		return "", errors.New("this order does not include a file download")
	}

	if order.DigitalProduct.FileURL == nil || strings.TrimSpace(*order.DigitalProduct.FileURL) == "" {
		return "", errors.New("file is not available for this product")
	}

	return *order.DigitalProduct.FileURL, nil
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
		if !isValidHTTPSURL(*p.AccessRedirectURL) {
			return errors.New("access_redirect_url must be a valid https URL")
		}
	case models.DigitalFulfillmentModeExternalLink:
		if p.AccessRedirectURL == nil || strings.TrimSpace(*p.AccessRedirectURL) == "" {
			return errors.New("cannot publish an external link product without an access_redirect_url")
		}
		if !isValidHTTPSURL(*p.AccessRedirectURL) {
			return errors.New("access_redirect_url must be a valid https URL")
		}
	case models.DigitalFulfillmentModeManualDelivery:
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

func (s *Service) buildDefaultCheckoutCallback(reference string) string {
	u := fmt.Sprintf("%s/public/digital-store/payment/callback", strings.TrimRight(s.frontendURL, "/"))
	if reference != "" {
		u = u + "?reference=" + url.QueryEscape(reference)
	}
	return u
}

func (s *Service) buildCompletionURL(orderID uuid.UUID, token string) string {
	return fmt.Sprintf(
		"%s/public/digital-orders/%s/complete?token=%s",
		strings.TrimRight(s.frontendURL, "/"),
		orderID.String(),
		url.QueryEscape(token),
	)
}

func (s *Service) generatePaymentReference() string {
	return "dig_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func (s *Service) generateUniqueProductCode() string {
	for {
		code := "prd_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

		var count int64
		if err := s.db.Model(&models.DigitalProduct{}).Where("slug = ?", code).Count(&count).Error; err != nil {
			return code
		}
		if count == 0 {
			return code
		}
	}
}

func (s *Service) paystackClient() (*paystack.Client, error) {
	secret := strings.TrimSpace(os.Getenv("PAYSTACK_SECRET_KEY"))
	if secret == "" {
		return nil, errors.New("PAYSTACK_SECRET_KEY is not set")
	}
	return paystack.NewClient(secret), nil
}

func (s *Service) getOwnerContactTx(tx *gorm.DB, businessID uuid.UUID) (*ownerContact, error) {
	var owner ownerContact

	err := tx.
		Table("business_users").
		Select(`
			users.id as user_id,
			users.email as email,
			COALESCE(users.first_name, 'there') as first_name
		`).
		Joins("JOIN users ON users.id = business_users.user_id").
		Where("business_users.business_id = ? AND business_users.role = ? AND business_users.is_active = ?", businessID, "owner", true).
		Limit(1).
		Scan(&owner).Error
	if err != nil {
		return nil, err
	}

	if owner.UserID == uuid.Nil || strings.TrimSpace(owner.Email) == "" {
		return nil, errors.New("active business owner email not found")
	}

	return &owner, nil
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

func isValidHTTPSURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https") && strings.TrimSpace(u.Host) != ""
}
