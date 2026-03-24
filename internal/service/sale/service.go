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

type CreateItemRequest struct {
	ProductID   *uuid.UUID `json:"product_id"`
	ProductName string     `json:"product_name" binding:"required"`
	ProductSKU  *string    `json:"product_sku"`
	UnitPrice   int64      `json:"unit_price" binding:"required,min=1"`
	Quantity    int        `json:"quantity" binding:"required,min=1"`
	Discount    int64      `json:"discount"`
}

type CreateRequest struct {
	StoreID        *uuid.UUID          `json:"store_id"`
	CustomerID     *uuid.UUID          `json:"customer_id"`
	InvoiceID      *uuid.UUID          `json:"invoice_id"`
	StaffName      *string             `json:"staff_name"`
	Items          []CreateItemRequest `json:"items" binding:"required,min=1"`
	PaymentMethod  string              `json:"payment_method" binding:"required"`
	AmountPaid     int64               `json:"amount_paid"`
	DiscountAmount int64               `json:"discount_amount"`
	VATRate        float64             `json:"vat_rate"`
	VATInclusive   bool                `json:"vat_inclusive"`
	WHTRate        float64             `json:"wht_rate"`
	Notes          *string             `json:"notes"`
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

// RecordPaymentRequest is used for POST /sales/:id/payment
type RecordPaymentRequest struct {
	Amount        int64  `json:"amount" binding:"required,min=1"` // kobo
	PaymentMethod string `json:"payment_method"`                  // e.g. "cash", "transfer"
	Note          string `json:"note"`
}

// ─── Create ──────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Sale, error) {
	// Generate sale number atomically — safe under concurrent requests from
	// multiple stores belonging to the same business.
	saleNumber, err := s.nextSaleNumber(businessID)
	if err != nil {
		return nil, err
	}

	// Build line items and accumulate sub-total
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

	sale := models.Sale{
		BusinessID:     businessID,
		StoreID:        req.StoreID,
		CustomerID:     req.CustomerID,
		InvoiceID:      req.InvoiceID,
		RecordedBy:     recordedBy,
		SaleNumber:     saleNumber,
		StaffName:      req.StaffName,
		SubTotal:       subTotal,
		DiscountAmount: req.DiscountAmount,
		VATRate:        req.VATRate,
		VATInclusive:   req.VATInclusive,
		WHTRate:        req.WHTRate,
		PaymentMethod:  req.PaymentMethod,
		Notes:          req.Notes,
	}

	// Derive VAT, WHT, and grand total
	sale.CalculateTotal()

	// Cap amount paid at total — prevents over-payment on creation
	sale.AmountPaid = req.AmountPaid
	if sale.AmountPaid > sale.TotalAmount {
		sale.AmountPaid = sale.TotalAmount
	}
	sale.BalanceDue = sale.TotalAmount - sale.AmountPaid
	if sale.BalanceDue < 0 {
		sale.BalanceDue = 0
	}

	switch {
	case sale.BalanceDue <= 0:
		sale.Status = models.SaleStatusCompleted
	default:
		sale.Status = models.SaleStatusPartial
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Persist sale
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}

		// 2. Persist line items
		for i := range items {
			items[i].SaleID = sale.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}

		// 3. Decrement inventory for tracked products
		for _, item := range req.Items {
			if item.ProductID != nil {
				tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", item.Quantity))
			}
		}

		// 4. Update customer purchase stats
		if req.CustomerID != nil {
			tx.Model(&models.Customer{}).Where("id = ?", *req.CustomerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("total_purchases + ?", sale.TotalAmount),
				"total_orders":    gorm.Expr("total_orders + 1"),
			})

			// 5. Auto-create a debt record for any unpaid balance
			if sale.BalanceDue > 0 {
				debt := models.Debt{
					BusinessID:  businessID,
					Direction:   models.DebtDirectionReceivable,
					CustomerID:  req.CustomerID,
					Description: fmt.Sprintf("Balance from sale %s", saleNumber),
					TotalAmount: sale.BalanceDue,
					AmountDue:   sale.BalanceDue,
					Status:      models.DebtStatusOutstanding,
					RecordedBy:  recordedBy,
				}
				if err := tx.Create(&debt).Error; err != nil {
					return err
				}
				tx.Model(&models.Customer{}).Where("id = ?", *req.CustomerID).
					UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt + ?", sale.BalanceDue))
			}
		}

		// 6. Ledger entry for the amount paid now (if any)
		if req.AmountPaid > 0 {
			ledger := models.LedgerEntry{
				BusinessID:  businessID,
				Type:        models.LedgerCredit,
				SourceType:  models.LedgerSourceSale,
				SourceID:    sale.ID,
				Amount:      req.AmountPaid,
				Balance:     0,
				Description: "Sale " + saleNumber,
				RecordedBy:  recordedBy,
			}
			return tx.Create(&ledger).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sale.SaleItems = items
	return &sale, nil
}

// ─── RecordPayment ────────────────────────────────────────────────────────────

// RecordPayment records a (partial or full) payment against an existing sale.
// Safe to call multiple times — each call reduces balance_due until it reaches 0.
// Also keeps the linked debt record and customer outstanding_debt in sync.
func (s *Service) RecordPayment(businessID, saleID, recordedBy uuid.UUID, req RecordPaymentRequest) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("Customer").First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}

	if sale.BalanceDue <= 0 {
		return nil, errors.New("sale is already fully paid")
	}
	if req.Amount > sale.BalanceDue {
		return nil, fmt.Errorf("payment of ₦%.2f exceeds balance due of ₦%.2f",
			float64(req.Amount)/100, float64(sale.BalanceDue)/100)
	}

	prevBalance := sale.BalanceDue
	sale.AmountPaid += req.Amount
	sale.BalanceDue -= req.Amount
	if sale.BalanceDue <= 0 {
		sale.BalanceDue = 0
		sale.Status = models.SaleStatusCompleted
	} else {
		sale.Status = models.SaleStatusPartial
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Update the sale record
		if err := tx.Model(&sale).Updates(map[string]interface{}{
			"amount_paid": sale.AmountPaid,
			"balance_due": sale.BalanceDue,
			"status":      sale.Status,
		}).Error; err != nil {
			return err
		}

		// 2. Sync the linked debt record (auto-created at sale time)
		var debt models.Debt
		debtDesc := fmt.Sprintf("Balance from sale %s", sale.SaleNumber)
		if err := tx.Where("business_id = ? AND description = ?", businessID, debtDesc).
			First(&debt).Error; err == nil {
			debt.AmountPaid += req.Amount
			debt.UpdateStatus()
			if err := tx.Model(&debt).Updates(map[string]interface{}{
				"amount_paid": debt.AmountPaid,
				"amount_due":  debt.AmountDue,
				"status":      debt.Status,
			}).Error; err != nil {
				return err
			}
			// Reduce the customer's tracked outstanding debt
			if debt.CustomerID != nil {
				tx.Model(&models.Customer{}).Where("id = ?", *debt.CustomerID).
					UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt - ?", req.Amount))
			}
		} else if sale.CustomerID != nil && prevBalance > 0 {
			// No debt record (walk-in sale that had a balance) — still update customer
			tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).
				UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt - ?", req.Amount))
		}

		// 3. Ledger entry for this payment
		note := req.Note
		if note == "" {
			note = fmt.Sprintf("Payment for sale %s", sale.SaleNumber)
		}
		ledger := models.LedgerEntry{
			BusinessID:  businessID,
			Type:        models.LedgerCredit,
			SourceType:  models.LedgerSourceSale,
			SourceID:    sale.ID,
			Amount:      req.Amount,
			Balance:     0,
			Description: note,
			RecordedBy:  recordedBy,
		}
		return tx.Create(&ledger).Error
	})
	if err != nil {
		return nil, err
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

// ─── List ─────────────────────────────────────────────────────────────────────

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
	err := q.Preload("Customer").
		Offset(offset).
		Limit(filter.Limit).
		Order("created_at DESC").
		Find(&sales).Error
	return sales, total, err
}

// ─── GenerateReceipt ──────────────────────────────────────────────────────────

func (s *Service) GenerateReceipt(businessID, saleID uuid.UUID) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").Preload("Customer").First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}

	// Only generate once — return existing receipt number if already issued
	if sale.ReceiptNumber != nil {
		return &sale, nil
	}

	receiptNumber, err := s.nextReceiptNumber(businessID)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(&sale).Update("receipt_number", receiptNumber).Error; err != nil {
		return nil, err
	}
	sale.ReceiptNumber = &receiptNumber
	return &sale, nil
}

// ─── Sequence helpers ─────────────────────────────────────────────────────────
//
// Both helpers use an atomic UPSERT on business_sale_sequences.
// The ON CONFLICT DO UPDATE holds a row-level lock for the duration of the
// increment, so concurrent calls from multiple stores are fully serialised
// at the database level — each caller gets a unique, gap-free number.

func (s *Service) nextSaleNumber(businessID uuid.UUID) (string, error) {
	var seq models.BusinessSaleSequence
	err := s.db.Raw(`
		INSERT INTO business_sale_sequences (business_id, last_sale_number, last_receipt_number)
		VALUES (?, 1, 0)
		ON CONFLICT (business_id)
		DO UPDATE SET last_sale_number = business_sale_sequences.last_sale_number + 1
		RETURNING last_sale_number
	`, businessID).Scan(&seq).Error
	if err != nil {
		return "", fmt.Errorf("could not generate sale number: %w", err)
	}
	return fmt.Sprintf("SL-%06d", seq.LastSaleNumber), nil
}

func (s *Service) nextReceiptNumber(businessID uuid.UUID) (string, error) {
	var seq models.BusinessSaleSequence
	err := s.db.Raw(`
		INSERT INTO business_sale_sequences (business_id, last_sale_number, last_receipt_number)
		VALUES (?, 0, 1)
		ON CONFLICT (business_id)
		DO UPDATE SET last_receipt_number = business_sale_sequences.last_receipt_number + 1
		RETURNING last_receipt_number
	`, businessID).Scan(&seq).Error
	if err != nil {
		return "", fmt.Errorf("could not generate receipt number: %w", err)
	}
	return fmt.Sprintf("RC-%06d", seq.LastReceiptNumber), nil
}
