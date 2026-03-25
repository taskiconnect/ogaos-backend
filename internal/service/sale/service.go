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
	AmountPaid     int64               `json:"amount_paid"` // 0 is valid — means full credit / pay-later
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
	PaymentMethod string `json:"payment_method"`
	Note          string `json:"note"`
}

// CancelRequest is used for PATCH /sales/:id/cancel
type CancelRequest struct {
	Reason string `json:"reason"` // optional — displayed to the business owner
}

// ─── Create ──────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Sale, error) {
	saleNumber, err := s.nextSaleNumber(businessID)
	if err != nil {
		return nil, err
	}

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

	sale.CalculateTotal()

	// Cap amount paid at total — prevents over-payment on creation.
	// Zero is valid: it means the customer hasn't paid anything yet (full credit).
	sale.AmountPaid = req.AmountPaid
	if sale.AmountPaid < 0 {
		sale.AmountPaid = 0
	}
	if sale.AmountPaid > sale.TotalAmount {
		sale.AmountPaid = sale.TotalAmount
	}
	sale.BalanceDue = sale.TotalAmount - sale.AmountPaid
	if sale.BalanceDue < 0 {
		sale.BalanceDue = 0
	}

	// Status logic:
	//   completed  → fully paid (balance = 0)
	//   partial    → some or no payment made (balance > 0)
	if sale.BalanceDue <= 0 {
		sale.Status = models.SaleStatusCompleted
	} else {
		sale.Status = models.SaleStatusPartial
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}

		for i := range items {
			items[i].SaleID = sale.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}

		// Decrement inventory for tracked products
		for _, item := range req.Items {
			if item.ProductID != nil {
				tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", item.Quantity))
			}
		}

		// Update customer purchase stats
		if req.CustomerID != nil {
			tx.Model(&models.Customer{}).Where("id = ?", *req.CustomerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("total_purchases + ?", sale.TotalAmount),
				"total_orders":    gorm.Expr("total_orders + 1"),
			})

			// Auto-create a debt record for any unpaid balance (including full zero-payment)
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

		// Ledger entry only if something was actually paid now
		if sale.AmountPaid > 0 {
			ledger := models.LedgerEntry{
				BusinessID:  businessID,
				Type:        models.LedgerCredit,
				SourceType:  models.LedgerSourceSale,
				SourceID:    sale.ID,
				Amount:      sale.AmountPaid,
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

// ─── Cancel ──────────────────────────────────────────────────────────────────

// Cancel marks a sale as cancelled and fully reverses all its side-effects:
//   - Restores stock for every tracked product line item
//   - Reverses the customer's total_purchases, total_orders, and outstanding_debt
//   - Cancels the linked debt record (if any) so it no longer appears as owed
//   - Posts a debit ledger entry to reverse any revenue already recorded
//
// A cancelled sale is never hard-deleted — it stays visible to the business
// owner so they can see who cancelled it and when. It is excluded from all
// revenue and total-sales aggregations via the status = 'cancelled' filter.
func (s *Service) Cancel(businessID, saleID, cancelledBy uuid.UUID, req CancelRequest) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").
		Preload("Customer").
		First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}

	if sale.Status == models.SaleStatusCancelled {
		return nil, errors.New("sale is already cancelled")
	}

	// Build a cancellation note that preserves who did it and why
	reason := req.Reason
	if reason == "" {
		reason = "no reason given"
	}
	cancelNote := fmt.Sprintf("Cancelled by staff (user %s): %s", cancelledBy.String(), reason)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Mark sale cancelled and record who/why
		if err := tx.Model(&sale).Updates(map[string]interface{}{
			"status": models.SaleStatusCancelled,
			"notes":  cancelNote,
		}).Error; err != nil {
			return err
		}
		sale.Status = models.SaleStatusCancelled

		// 2. Restore inventory for every tracked product in the sale
		for _, item := range sale.SaleItems {
			if item.ProductID != nil {
				tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity + ?", item.Quantity))
			}
		}

		// 3. Reverse customer stats and linked debt
		if sale.CustomerID != nil {
			// Roll back purchase totals
			tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("GREATEST(total_purchases - ?, 0)", sale.TotalAmount),
				"total_orders":    gorm.Expr("GREATEST(total_orders - 1, 0)"),
			})

			// Cancel the linked debt record so it stops showing as outstanding
			debtDesc := fmt.Sprintf("Balance from sale %s", sale.SaleNumber)
			var debt models.Debt
			if err := tx.Where("business_id = ? AND description = ? AND status != ?",
				businessID, debtDesc, models.DebtStatusSettled).
				First(&debt).Error; err == nil {
				remainingDue := debt.AmountDue
				if err := tx.Model(&debt).Updates(map[string]interface{}{
					"status":     "cancelled",
					"amount_due": 0,
				}).Error; err != nil {
					return err
				}
				// Reduce outstanding_debt by whatever was still owed
				if remainingDue > 0 {
					tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).
						UpdateColumn("outstanding_debt", gorm.Expr("GREATEST(outstanding_debt - ?, 0)", remainingDue))
				}
			}
		}

		// 4. If any money was already collected, post a reversing debit ledger entry
		//    so the revenue figure is corrected automatically.
		if sale.AmountPaid > 0 {
			reversal := models.LedgerEntry{
				BusinessID:  businessID,
				Type:        models.LedgerDebit, // debit = reversal of income
				SourceType:  models.LedgerSourceSale,
				SourceID:    sale.ID,
				Amount:      sale.AmountPaid,
				Balance:     0,
				Description: fmt.Sprintf("Reversal — sale %s cancelled", sale.SaleNumber),
				RecordedBy:  cancelledBy,
			}
			return tx.Create(&reversal).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &sale, nil
}

// ─── RecordPayment ────────────────────────────────────────────────────────────

func (s *Service) RecordPayment(businessID, saleID, recordedBy uuid.UUID, req RecordPaymentRequest) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("Customer").First(&sale).Error; err != nil {
		return nil, errors.New("sale not found")
	}

	if sale.Status == models.SaleStatusCancelled {
		return nil, errors.New("cannot record payment on a cancelled sale")
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
		if err := tx.Model(&sale).Updates(map[string]interface{}{
			"amount_paid": sale.AmountPaid,
			"balance_due": sale.BalanceDue,
			"status":      sale.Status,
		}).Error; err != nil {
			return err
		}

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
			if debt.CustomerID != nil {
				tx.Model(&models.Customer{}).Where("id = ?", *debt.CustomerID).
					UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt - ?", req.Amount))
			}
		} else if sale.CustomerID != nil && prevBalance > 0 {
			tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).
				UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt - ?", req.Amount))
		}

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
