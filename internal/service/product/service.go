package product

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/cursor"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type CreateRequest struct {
	Name              string     `json:"name" binding:"required"`
	Description       *string    `json:"description"`
	Type              string     `json:"type" binding:"required"` // product | service
	SKU               *string    `json:"sku"`
	Price             int64      `json:"price" binding:"required,min=1"`
	CostPrice         *int64     `json:"cost_price"`
	StoreID           *uuid.UUID `json:"store_id"`
	TrackInventory    bool       `json:"track_inventory"`
	StockQuantity     int        `json:"stock_quantity"`
	LowStockThreshold int        `json:"low_stock_threshold"`
}

type UpdateRequest struct {
	Name              *string    `json:"name"`
	Description       *string    `json:"description"`
	SKU               *string    `json:"sku"`
	Price             *int64     `json:"price"`
	CostPrice         *int64     `json:"cost_price"`
	StoreID           *uuid.UUID `json:"store_id"`
	TrackInventory    *bool      `json:"track_inventory"`
	LowStockThreshold *int       `json:"low_stock_threshold"`
	IsActive          *bool      `json:"is_active"`
}

type AdjustStockRequest struct {
	Quantity int    `json:"quantity" binding:"required"` // positive = add, negative = remove
	Reason   string `json:"reason"`
}

type ListFilter struct {
	StoreID  *uuid.UUID
	Type     string
	Search   string
	LowStock bool
	Cursor   string
	Limit    int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID uuid.UUID, req CreateRequest, idempotencyKey string) (*models.Product, error) {
	if req.Type != models.ProductTypeProduct && req.Type != models.ProductTypeService {
		return nil, errors.New("type must be 'product' or 'service'")
	}

	var parsedKey *uuid.UUID
	if strings.TrimSpace(idempotencyKey) != "" {
		if key, err := uuid.Parse(strings.TrimSpace(idempotencyKey)); err == nil {
			parsedKey = &key

			var existing models.Product
			err := s.db.
				Where("business_id = ? AND idempotency_key = ? AND created_at > ?", businessID, key, time.Now().UTC().Add(-24*time.Hour)).
				First(&existing).Error

			if err == nil {
				return &existing, nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
	}

	threshold := req.LowStockThreshold
	if threshold == 0 {
		threshold = 5
	}

	p := models.Product{
		BusinessID:        businessID,
		StoreID:           req.StoreID,
		Name:              req.Name,
		Description:       req.Description,
		Type:              req.Type,
		SKU:               req.SKU,
		Price:             req.Price,
		CostPrice:         req.CostPrice,
		TrackInventory:    req.TrackInventory,
		StockQuantity:     req.StockQuantity,
		LowStockThreshold: threshold,
		IdempotencyKey:    parsedKey,
	}

	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) Get(businessID, productID uuid.UUID) (*models.Product, error) {
	var p models.Product
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}
	return &p, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Product, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	q := s.db.Model(&models.Product{}).Where("business_id = ? AND is_active = true", businessID)
	if filter.StoreID != nil {
		q = q.Where("store_id = ?", *filter.StoreID)
	}
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("name ILIKE ? OR sku ILIKE ?", like, like)
	}
	if filter.LowStock {
		q = q.Where("track_inventory = true AND stock_quantity <= low_stock_threshold")
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cur, id)
	}

	var products []models.Product
	if err := q.Order("created_at DESC, id DESC").Limit(filter.Limit + 1).Find(&products).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(products) > filter.Limit {
		last := products[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		products = products[:filter.Limit]
	}
	return products, nextCursor, nil
}

func (s *Service) Update(businessID, productID uuid.UUID, req UpdateRequest) (*models.Product, error) {
	var p models.Product
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.SKU != nil {
		updates["sku"] = *req.SKU
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.CostPrice != nil {
		updates["cost_price"] = *req.CostPrice
	}
	if req.StoreID != nil {
		updates["store_id"] = *req.StoreID
	}
	if req.TrackInventory != nil {
		updates["track_inventory"] = *req.TrackInventory
	}
	if req.LowStockThreshold != nil {
		updates["low_stock_threshold"] = *req.LowStockThreshold
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := s.db.Model(&p).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// AdjustStock adds or subtracts from stock_quantity.
func (s *Service) AdjustStock(businessID, productID uuid.UUID, req AdjustStockRequest) (*models.Product, error) {
	var p models.Product
	if err := s.db.Where("id = ? AND business_id = ?", productID, businessID).First(&p).Error; err != nil {
		return nil, errors.New("product not found")
	}
	if !p.TrackInventory {
		return nil, errors.New("inventory tracking is not enabled for this product")
	}
	newQty := p.StockQuantity + req.Quantity
	if newQty < 0 {
		return nil, errors.New("adjustment would result in negative stock")
	}
	if err := s.db.Model(&p).Update("stock_quantity", newQty).Error; err != nil {
		return nil, err
	}
	p.StockQuantity = newQty
	return &p, nil
}

func (s *Service) UpdateImage(businessID, productID uuid.UUID, imageURL string) error {
	result := s.db.Model(&models.Product{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Update("image_url", imageURL)
	if result.RowsAffected == 0 {
		return errors.New("product not found")
	}
	return result.Error
}

func (s *Service) Delete(businessID, productID uuid.UUID) error {
	result := s.db.Model(&models.Product{}).
		Where("id = ? AND business_id = ?", productID, businessID).
		Update("is_active", false)
	if result.RowsAffected == 0 {
		return errors.New("product not found")
	}
	return result.Error
}
