// internal/service/sale/service.go
package sale

import (
	"errors"
	"fmt"
	"strings"
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
	Quantity    int        `json:"quantity"   binding:"required,min=1"`
	Discount    int64      `json:"discount"`
}

// WalkInCustomer is provided inline when the customer is new.
// The service creates a Customer record and links it to the sale.
type WalkInCustomer struct {
	FirstName string  `json:"first_name" binding:"required"`
	LastName  string  `json:"last_name"`
	Phone     string  `json:"phone"      binding:"required"`
	Email     *string `json:"email"`
}

type CreateRequest struct {
	StoreID          *uuid.UUID      `json:"store_id"`
	CustomerID       *uuid.UUID      `json:"customer_id"`
	InvoiceID        *uuid.UUID      `json:"invoice_id"`
	WalkIn           *WalkInCustomer `json:"walk_in_customer"`
	StaffName        *string         `json:"staff_name"`
	Items            []SaleItemInput `json:"items"           binding:"required,min=1"`
	DiscountAmount   int64           `json:"discount_amount"`
	VATRate          float64         `json:"vat_rate"`
	VATInclusive     bool            `json:"vat_inclusive"`
	WHTRate          float64         `json:"wht_rate"`
	PaymentMethod    string          `json:"payment_method"  binding:"required"`
	AmountPaid       int64           `json:"amount_paid"`
	Notes            *string         `json:"notes"`
	SendReceiptEmail bool            `json:"send_receipt_email"`
}

type ListFilter struct {
	StoreID    *uuid.UUID
	CustomerID *uuid.UUID
	Status     string
	DateFrom   *time.Time
	DateTo     *time.Time
	Cursor     string
	Limit      int
}

// ─── Create ──────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Sale, error) {
	saleNumber, err := s.nextSaleNumber(businessID)
	if err != nil {
		return nil, err
	}

	// ── Resolve customer ──────────────────────────────────────────────────────
	// Priority: explicit customer_id > walk_in match by phone > create new
	customerID := req.CustomerID

	if customerID == nil && req.WalkIn != nil {
		phone := strings.TrimSpace(req.WalkIn.Phone)

		// Try to find by phone first
		var existing models.Customer
		err := s.db.Where("business_id = ? AND phone_number = ?", businessID, phone).
			First(&existing).Error

		if err == nil {
			// Found — reuse
			customerID = &existing.ID
		} else {
			// Create new customer so they appear in search next time
			newCust := models.Customer{
				BusinessID:  businessID,
				FirstName:   strings.TrimSpace(req.WalkIn.FirstName),
				LastName:    strings.TrimSpace(req.WalkIn.LastName),
				Email:       req.WalkIn.Email,
				PhoneNumber: &phone,
				IsActive:    true,
			}
			if err := s.db.Create(&newCust).Error; err != nil {
				return nil, fmt.Errorf("create customer: %w", err)
			}
			customerID = &newCust.ID
		}
	}

	// ── Build sale items ──────────────────────────────────────────────────────
	var items []models.SaleItem
	var subTotal int64
	for _, item := range req.Items {
		lineTotal := (item.UnitPrice * int64(item.Quantity)) - item.Discount
		if lineTotal < 0 {
			lineTotal = 0
		}
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

	// ── Build sale ────────────────────────────────────────────────────────────
	amountPaid := req.AmountPaid
	if amountPaid < 0 {
		amountPaid = 0
	}

	sale := models.Sale{
		BusinessID:     businessID,
		StoreID:        req.StoreID,
		CustomerID:     customerID,
		InvoiceID:      req.InvoiceID,
		RecordedBy:     recordedBy,
		StaffName:      req.StaffName,
		SaleNumber:     saleNumber,
		SubTotal:       subTotal,
		DiscountAmount: req.DiscountAmount,
		VATRate:        req.VATRate,
		VATInclusive:   req.VATInclusive,
		WHTRate:        req.WHTRate,
		PaymentMethod:  req.PaymentMethod,
		AmountPaid:     amountPaid,
		Notes:          req.Notes,
	}
	sale.CalculateVAT()
	sale.CalculateWHT()
	sale.CalculateTotal() // also sets BalanceDue = TotalAmount - AmountPaid

	// ── Determine status ──────────────────────────────────────────────────────
	switch {
	case amountPaid == 0:
		sale.Status = models.SaleStatusPending
	case sale.BalanceDue > 0:
		sale.Status = models.SaleStatusPartial
	default:
		sale.Status = models.SaleStatusCompleted
	}

	// ── Persist in one transaction ────────────────────────────────────────────
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}

		// Attach sale ID to items
		for i := range items {
			items[i].SaleID = sale.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}

		// Deduct inventory
		for _, item := range req.Items {
			if item.ProductID != nil {
				tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", item.Quantity))
			}
		}

		// Update customer totals
		if customerID != nil {
			tx.Model(&models.Customer{}).Where("id = ?", *customerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("total_purchases + ?", sale.TotalAmount),
				"total_orders":    gorm.Expr("total_orders + 1"),
			})
		}

		// Ledger entry (only for amount actually paid)
		if amountPaid > 0 {
			ledger := models.LedgerEntry{
				BusinessID:  businessID,
				Type:        models.LedgerCredit,
				SourceType:  models.LedgerSourceSale,
				SourceID:    sale.ID,
				Amount:      amountPaid,
				Balance:     0,
				Description: "Sale " + saleNumber,
				RecordedBy:  recordedBy,
			}
			if err := tx.Create(&ledger).Error; err != nil {
				return err
			}
		}

		// Auto-create debt for balance due
		if sale.BalanceDue > 0 && customerID != nil {
			debt := models.Debt{
				BusinessID:  businessID,
				Direction:   models.DebtDirectionReceivable,
				CustomerID:  customerID,
				Description: fmt.Sprintf("Balance from sale %s", saleNumber),
				TotalAmount: sale.BalanceDue,
				AmountDue:   sale.BalanceDue,
				Status:      models.DebtStatusOutstanding,
				RecordedBy:  recordedBy,
			}
			if err := tx.Create(&debt).Error; err != nil {
				return err
			}
			// Update customer outstanding_debt
			tx.Model(&models.Customer{}).Where("id = ?", *customerID).
				UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt + ?", sale.BalanceDue))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sale.SaleItems = items

	// Reload customer so it comes back in the response
	if customerID != nil {
		var cust models.Customer
		if err := s.db.First(&cust, "id = ?", *customerID).Error; err == nil {
			sale.Customer = &cust
		}
	}

	return &sale, nil
}

// ─── Get ─────────────────────────────────────────────────────────────────────

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

// ─── List (cursor-paginated) ──────────────────────────────────────────────────

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Sale, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

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
	if filter.Cursor != "" {
		// cursor is the created_at timestamp of the last seen record (ISO8601)
		q = q.Where("created_at < ?", filter.Cursor)
	}

	var sales []models.Sale
	err := q.
		Preload("Customer").
		Preload("SaleItems").
		Order("created_at DESC").
		Limit(filter.Limit + 1).
		Find(&sales).Error
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(sales) > filter.Limit {
		nextCursor = sales[filter.Limit-1].CreatedAt.UTC().Format(time.RFC3339Nano)
		sales = sales[:filter.Limit]
	}

	return sales, nextCursor, nil
}

// ─── GenerateReceipt ─────────────────────────────────────────────────────────

func (s *Service) GenerateReceipt(businessID, saleID uuid.UUID) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").Preload("Customer").
		First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}

	if sale.ReceiptNumber == nil {
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
		sale.ReceiptSentAt = &now
	}

	return &sale, nil
}

// ─── Number helpers ───────────────────────────────────────────────────────────

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
