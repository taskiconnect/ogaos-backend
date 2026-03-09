// internal/service/sale/service.go
package sale

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type SaleItemInput struct {
	ProductID   *uuid.UUID `json:"product_id"`
	ProductName string     `json:"product_name" binding:"required"`
	ProductSKU  *string    `json:"product_sku"`
	UnitPrice   int64      `json:"unit_price" binding:"required,min=1"`
	Quantity    int        `json:"quantity" binding:"required,min=1"`
	Discount    int64      `json:"discount"`
}

type CreateRequest struct {
	StoreID        *uuid.UUID      `json:"store_id"`
	CustomerID     *uuid.UUID      `json:"customer_id"`
	InvoiceID      *uuid.UUID      `json:"invoice_id"`
	Items          []SaleItemInput `json:"items" binding:"required,min=1"`
	DiscountAmount int64           `json:"discount_amount"`
	VATRate        float64         `json:"vat_rate"`
	VATInclusive   bool            `json:"vat_inclusive"`
	WHTRate        float64         `json:"wht_rate"`
	PaymentMethod  string          `json:"payment_method" binding:"required"`
	Notes          *string         `json:"notes"`
}

type RecordPaymentRequest struct {
	Amount        int64  `json:"amount" binding:"required,min=1"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

type ListFilter struct {
	StoreID    *uuid.UUID
	CustomerID *uuid.UUID
	Status     string
	DateFrom   *time.Time
	DateTo     *time.Time
	Page       int
	Limit      int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// Create records a new completed sale with line items.
// Deducts stock for tracked inventory products.
// Creates a ledger entry for the revenue.
func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Sale, error) {
	saleNumber, err := s.nextSaleNumber(businessID)
	if err != nil {
		return nil, err
	}

	// Build sale items and compute subtotal
	var items []models.SaleItem
	var subTotal int64
	for _, item := range req.Items {
		lineTotal := (item.UnitPrice * int64(item.Quantity)) - item.Discount
		items = append(items, models.SaleItem{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			ProductSKU:  item.ProductSKU,
			UnitPrice:   item.UnitPrice,
			Quantity:    item.Quantity,
			Discount:    item.Discount,
			TotalPrice:  lineTotal,
		})
		subTotal += lineTotal
	}

	sale := models.Sale{
		BusinessID:     businessID,
		StoreID:        req.StoreID,
		CustomerID:     req.CustomerID,
		InvoiceID:      req.InvoiceID,
		RecordedBy:     recordedBy,
		SaleNumber:     saleNumber,
		SubTotal:       subTotal,
		DiscountAmount: req.DiscountAmount,
		VATRate:        req.VATRate,
		VATInclusive:   req.VATInclusive,
		WHTRate:        req.WHTRate,
		PaymentMethod:  req.PaymentMethod,
		Status:         models.SaleStatusCompleted,
		Notes:          req.Notes,
	}
	sale.CalculateVAT()
	sale.CalculateWHT()
	sale.CalculateTotal()

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}

		// Assign sale ID to items and save
		for i := range items {
			items[i].SaleID = sale.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}

		// Deduct stock for tracked products
		for _, item := range req.Items {
			if item.ProductID != nil {
				tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", item.Quantity))
			}
		}

		// Update customer stats
		if req.CustomerID != nil {
			tx.Model(&models.Customer{}).Where("id = ?", *req.CustomerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("total_purchases + ?", sale.TotalAmount),
				"total_orders":    gorm.Expr("total_orders + 1"),
			})
		}

		// Ledger entry — revenue
		ledger := models.LedgerEntry{
			BusinessID:  businessID,
			Type:        models.LedgerCredit,
			SourceType:  models.LedgerSourceSale,
			SourceID:    sale.ID,
			Amount:      sale.TotalAmount,
			Balance:     0, // running balance computed by ledger query
			Description: "Sale " + saleNumber,
			RecordedBy:  recordedBy,
		}
		return tx.Create(&ledger).Error
	})
	if err != nil {
		return nil, err
	}

	sale.SaleItems = items
	return &sale, nil
}

func (s *Service) Get(businessID, saleID uuid.UUID) (*models.Sale, error) {
	var sale models.Sale
	err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").
		Preload("Customer").
		First(&sale).Error
	if err != nil {
		return nil, errors.New("sale not found")
	}
	return &sale, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Sale, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit

	q := s.db.Model(&models.Sale{}).Where("business_id = ?", businessID)
	if filter.StoreID != nil {
		q = q.Where("store_id = ?", *filter.StoreID)
	}
	if filter.CustomerID != nil {
		q = q.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.DateFrom != nil {
		q = q.Where("created_at >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		q = q.Where("created_at <= ?", *filter.DateTo)
	}

	var total int64
	q.Count(&total)

	var sales []models.Sale
	err := q.Preload("Customer").Offset(offset).Limit(filter.Limit).Order("created_at DESC").Find(&sales).Error
	return sales, total, err
}

// GenerateReceipt assigns a receipt number to a completed sale.
// Idempotent — safe to call multiple times.
func (s *Service) GenerateReceipt(businessID, saleID uuid.UUID) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}
	if sale.ReceiptNumber != nil {
		return &sale, nil // already has receipt number
	}

	receiptNumber, err := s.nextReceiptNumber(businessID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.db.Model(&sale).Updates(map[string]interface{}{
		"receipt_number":  receiptNumber,
		"receipt_sent_at": now,
	}).Error; err != nil {
		return nil, err
	}
	sale.ReceiptNumber = &receiptNumber
	return &sale, nil
}

// ─── Number generation ────────────────────────────────────────────────────────

func (s *Service) nextSaleNumber(businessID uuid.UUID) (string, error) {
	prefix := fmt.Sprintf("SL-%s-", time.Now().Format("200601"))
	var last models.Sale
	err := s.db.Where("business_id = ? AND sale_number LIKE ?", businessID, prefix+"%").
		Order("sale_number DESC").First(&last).Error

	seq := 1
	if err == nil {
		fmt.Sscanf(last.SaleNumber[len(prefix):], "%d", &seq)
		seq++
	}
	return fmt.Sprintf("%s%04d", prefix, seq), nil
}

func (s *Service) nextReceiptNumber(businessID uuid.UUID) (string, error) {
	prefix := fmt.Sprintf("RCT-%s-", time.Now().Format("200601"))
	var last models.Sale
	err := s.db.Where("business_id = ? AND receipt_number LIKE ?", businessID, prefix+"%").
		Order("receipt_number DESC").First(&last).Error

	seq := 1
	if err == nil && last.ReceiptNumber != nil {
		fmt.Sscanf((*last.ReceiptNumber)[len(prefix):], "%d", &seq)
		seq++
	}
	return fmt.Sprintf("%s%04d", prefix, seq), nil
}
