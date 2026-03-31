package sale

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	apperr "ogaos-backend/internal/pkg/errors"
)

type ReceiptEmailPayload struct {
	SaleID            uuid.UUID
	BusinessID        uuid.UUID
	ToEmail           string
	CustomerFirstName string
	CustomerLastName  string
	SaleNumber        string
	ReceiptNumber     string
	PaymentMethod     string
	AmountPaid        int64
	BalanceDue        int64
	TotalAmount       int64
	CreatedAt         time.Time
	Items             []models.SaleItem
	Notes             *string
}

type ReceiptSender interface {
	SendSaleReceipt(payload ReceiptEmailPayload) error
}

type Service struct {
	db            *gorm.DB
	receiptSender ReceiptSender
}

func NewService(db *gorm.DB, receiptSender ReceiptSender) *Service {
	return &Service{
		db:            db,
		receiptSender: receiptSender,
	}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type WalkInCustomer struct {
	FirstName string  `json:"first_name" binding:"required"`
	LastName  string  `json:"last_name"`
	Phone     string  `json:"phone" binding:"required"`
	Email     *string `json:"email"`
}

type CreateItemRequest struct {
	ProductID   *uuid.UUID `json:"product_id"`
	ProductName string     `json:"product_name" binding:"required"`
	ProductSKU  *string    `json:"product_sku"`
	UnitPrice   int64      `json:"unit_price" binding:"required,min=1"`
	Quantity    int        `json:"quantity" binding:"required,min=1"`
	Discount    int64      `json:"discount"`
}

type CreateRequest struct {
	StoreID          *uuid.UUID          `json:"store_id"`
	CustomerID       *uuid.UUID          `json:"customer_id"`
	WalkInCustomer   *WalkInCustomer     `json:"walk_in_customer"`
	InvoiceID        *uuid.UUID          `json:"invoice_id"`
	StaffName        *string             `json:"staff_name"`
	Items            []CreateItemRequest `json:"items" binding:"required,min=1"`
	PaymentMethod    string              `json:"payment_method" binding:"required"`
	AmountPaid       int64               `json:"amount_paid"`
	DiscountAmount   int64               `json:"discount_amount"`
	VATRate          float64             `json:"vat_rate"`
	VATInclusive     bool                `json:"vat_inclusive"`
	WHTRate          float64             `json:"wht_rate"`
	Notes            *string             `json:"notes"`
	SendReceiptEmail bool                `json:"send_receipt_email"`
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

type RecordPaymentRequest struct {
	Amount        int64  `json:"amount" binding:"required,min=1"`
	PaymentMethod string `json:"payment_method"`
	Note          string `json:"note"`
}

type CancelRequest struct {
	Reason string `json:"reason"`
}

// ─── DB error mapping ────────────────────────────────────────────────────────

func fromDB(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.Wrap(apperr.CodeNotFound, "sale not found", err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "sales_sale_number_key":
				return apperr.Wrap(
					apperr.CodeConflict,
					"This sale appears to have already been recorded. Please refresh and check your sales list.",
					err,
				)
			default:
				return apperr.Wrap(apperr.CodeConflict, "resource already exists", err)
			}
		case "23503":
			return apperr.Wrap(apperr.CodeBadRequest, "one or more selected records are invalid", err)
		case "23514":
			return apperr.Wrap(apperr.CodeBadRequest, "one or more submitted values are invalid", err)
		case "22P02":
			return apperr.Wrap(apperr.CodeBadRequest, "invalid input format", err)
		default:
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
	}

	return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
}

func normalizePaymentMethod(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
}

func normalizePhone(phone string) string {
	return strings.TrimSpace(phone)
}

func normalizeOptionalEmail(email *string) *string {
	if email == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*email)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOptionalString(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ─── Create ──────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest, idempotencyKey string) (*models.Sale, error) {
	if businessID == uuid.Nil {
		return nil, apperr.New(apperr.CodeBadRequest, "business is required")
	}
	if recordedBy == uuid.Nil {
		return nil, apperr.New(apperr.CodeBadRequest, "recorded by is required")
	}
	if len(req.Items) == 0 {
		return nil, apperr.New(apperr.CodeBadRequest, "at least one sale item is required")
	}

	// ── FIX 1: Normalise and validate walk-in fields BEFORE the transaction ──
	// Previously this happened inside the transaction closure, which meant
	// validation errors surfaced as DB errors and the normalised values were
	// not available to the receipt-sending code that runs after the transaction.
	if req.WalkInCustomer != nil {
		req.WalkInCustomer.FirstName = strings.TrimSpace(req.WalkInCustomer.FirstName)
		req.WalkInCustomer.LastName = strings.TrimSpace(req.WalkInCustomer.LastName)
		req.WalkInCustomer.Phone = normalizePhone(req.WalkInCustomer.Phone)
		req.WalkInCustomer.Email = normalizeOptionalEmail(req.WalkInCustomer.Email)

		if req.WalkInCustomer.FirstName == "" {
			return nil, apperr.New(apperr.CodeBadRequest, "walk-in customer first name is required")
		}
		if req.WalkInCustomer.Phone == "" {
			return nil, apperr.New(apperr.CodeBadRequest, "walk-in customer phone is required")
		}
	}

	req.PaymentMethod = normalizePaymentMethod(req.PaymentMethod)
	if req.PaymentMethod == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "payment method is required")
	}

	if req.DiscountAmount < 0 {
		return nil, apperr.New(apperr.CodeBadRequest, "discount amount cannot be negative")
	}
	if req.AmountPaid < 0 {
		return nil, apperr.New(apperr.CodeBadRequest, "amount paid cannot be negative")
	}
	if req.VATRate < 0 || req.WHTRate < 0 {
		return nil, apperr.New(apperr.CodeBadRequest, "tax rates cannot be negative")
	}
	if req.CustomerID != nil && req.WalkInCustomer != nil {
		return nil, apperr.New(apperr.CodeBadRequest, "provide either customer_id or walk_in_customer, not both")
	}

	var parsedKey *uuid.UUID
	trimmedKey := strings.TrimSpace(idempotencyKey)
	if trimmedKey != "" {
		key, err := uuid.Parse(trimmedKey)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeBadRequest, "invalid idempotency key", err)
		}
		parsedKey = &key

		var existing models.Sale
		err = s.db.
			Where("business_id = ? AND idempotency_key = ? AND created_at > ?", businessID, key, time.Now().UTC().Add(-24*time.Hour)).
			Preload("SaleItems").
			Preload("Customer").
			First(&existing).Error

		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fromDB(err)
		}
	}

	saleNumber, err := s.nextSaleNumber(businessID)
	if err != nil {
		return nil, err
	}

	var items []models.SaleItem
	var subTotal int64

	for _, item := range req.Items {
		if strings.TrimSpace(item.ProductName) == "" {
			return nil, apperr.New(apperr.CodeBadRequest, "each sale item must have a product name")
		}
		if item.UnitPrice < 1 {
			return nil, apperr.New(apperr.CodeBadRequest, "unit price must be greater than zero")
		}
		if item.Quantity < 1 {
			return nil, apperr.New(apperr.CodeBadRequest, "quantity must be at least 1")
		}
		if item.Discount < 0 {
			return nil, apperr.New(apperr.CodeBadRequest, "item discount cannot be negative")
		}

		lineTotal := (item.UnitPrice * int64(item.Quantity)) - item.Discount
		if lineTotal < 0 {
			lineTotal = 0
		}

		items = append(items, models.SaleItem{
			ProductID:   item.ProductID,
			ProductName: strings.TrimSpace(item.ProductName),
			ProductSKU:  item.ProductSKU,
			UnitPrice:   item.UnitPrice,
			Quantity:    item.Quantity,
			Discount:    item.Discount,
			TotalPrice:  lineTotal,
		})
		subTotal += lineTotal
	}

	var sale models.Sale
	var shouldSendReceipt bool

	// ── FIX 2: Track the resolved customer inside the transaction so the
	// receipt-sending code after the transaction has the full customer object
	// (including email) without needing an extra DB round-trip.
	var resolvedCustomer *models.Customer

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert walk-in customer. Fields are already normalised and validated
		// above, so we use them directly here.
		if req.CustomerID == nil && req.WalkInCustomer != nil {
			firstName := req.WalkInCustomer.FirstName
			lastName := req.WalkInCustomer.LastName
			phone := req.WalkInCustomer.Phone
			email := req.WalkInCustomer.Email

			var existingCustomer models.Customer
			findErr := tx.
				Where("business_id = ? AND phone_number = ?", businessID, phone).
				First(&existingCustomer).Error

			if findErr == nil {
				// Customer exists — patch any missing fields.
				updates := map[string]interface{}{}
				if strings.TrimSpace(existingCustomer.FirstName) == "" {
					updates["first_name"] = firstName
					existingCustomer.FirstName = firstName
				}
				if strings.TrimSpace(existingCustomer.LastName) == "" && lastName != "" {
					updates["last_name"] = lastName
					existingCustomer.LastName = lastName
				}
				if existingCustomer.Email == nil && email != nil {
					updates["email"] = *email
					existingCustomer.Email = email // reflect locally so receipt has the email
				}
				if len(updates) > 0 {
					if err := tx.Model(&existingCustomer).Updates(updates).Error; err != nil {
						return fromDB(err)
					}
				}
				req.CustomerID = &existingCustomer.ID
				resolvedCustomer = &existingCustomer
			} else if errors.Is(findErr, gorm.ErrRecordNotFound) {
				// Brand-new customer.
				customer := models.Customer{
					BusinessID:  businessID,
					FirstName:   firstName,
					LastName:    lastName,
					PhoneNumber: &phone,
					Email:       email,
					IsActive:    true,
				}
				if err := tx.Create(&customer).Error; err != nil {
					return fromDB(err)
				}
				req.CustomerID = &customer.ID
				resolvedCustomer = &customer
			} else {
				return fromDB(findErr)
			}
		}

		sale = models.Sale{
			BusinessID:     businessID,
			StoreID:        req.StoreID,
			CustomerID:     req.CustomerID,
			InvoiceID:      req.InvoiceID,
			RecordedBy:     recordedBy,
			SaleNumber:     saleNumber,
			StaffName:      normalizeOptionalString(req.StaffName),
			SubTotal:       subTotal,
			DiscountAmount: req.DiscountAmount,
			VATRate:        req.VATRate,
			VATInclusive:   req.VATInclusive,
			WHTRate:        req.WHTRate,
			PaymentMethod:  req.PaymentMethod,
			Notes:          normalizeOptionalString(req.Notes),
			IdempotencyKey: parsedKey,
		}

		sale.CalculateTotal()

		if req.AmountPaid > sale.TotalAmount {
			return apperr.New(apperr.CodeBadRequest, "amount paid cannot be more than total due")
		}

		sale.AmountPaid = req.AmountPaid
		sale.BalanceDue = sale.TotalAmount - sale.AmountPaid
		if sale.BalanceDue < 0 {
			sale.BalanceDue = 0
		}

		if sale.BalanceDue <= 0 {
			sale.Status = models.SaleStatusCompleted
		} else {
			sale.Status = models.SaleStatusPartial
		}

		if err := tx.Create(&sale).Error; err != nil {
			return fromDB(err)
		}

		for i := range items {
			items[i].SaleID = sale.ID
		}

		if err := tx.Create(&items).Error; err != nil {
			return fromDB(err)
		}

		for _, item := range req.Items {
			if item.ProductID != nil {
				if err := tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", item.Quantity)).Error; err != nil {
					return fromDB(err)
				}
			}
		}

		if req.CustomerID != nil {
			if err := tx.Model(&models.Customer{}).
				Where("id = ? AND business_id = ?", *req.CustomerID, businessID).
				Updates(map[string]interface{}{
					"total_purchases": gorm.Expr("total_purchases + ?", sale.TotalAmount),
					"total_orders":    gorm.Expr("total_orders + 1"),
				}).Error; err != nil {
				return fromDB(err)
			}

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
					return fromDB(err)
				}

				if err := tx.Model(&models.Customer{}).
					Where("id = ? AND business_id = ?", *req.CustomerID, businessID).
					UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt + ?", sale.BalanceDue)).Error; err != nil {
					return fromDB(err)
				}
			}
		}

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
			if err := tx.Create(&ledger).Error; err != nil {
				return fromDB(err)
			}
		}

		// ── FIX 3: Receipt email block ──
		// Previously this block loaded the customer from the DB again with a
		// plain DB handle (not the transaction), which could race or miss a
		// just-created walk-in customer whose INSERT hadn't committed yet.
		// Now we use the resolvedCustomer captured in the walk-in block above,
		// falling back to a tx-scoped query for existing (non-walk-in) customers.
		if req.SendReceiptEmail && req.CustomerID != nil {
			var emailAddr *string

			if resolvedCustomer != nil {
				// Walk-in path: we already have the customer object in memory.
				emailAddr = resolvedCustomer.Email
			} else {
				// Existing customer path: load within the transaction.
				var customer models.Customer
				if err := tx.Where("id = ? AND business_id = ?", *req.CustomerID, businessID).
					First(&customer).Error; err != nil {
					return fromDB(err)
				}
				resolvedCustomer = &customer
				emailAddr = customer.Email
			}

			if emailAddr != nil && strings.TrimSpace(*emailAddr) != "" {
				receiptNumber, err := s.nextReceiptNumberTx(tx, businessID)
				if err != nil {
					return err
				}
				if err := tx.Model(&sale).Update("receipt_number", receiptNumber).Error; err != nil {
					return fromDB(err)
				}
				sale.ReceiptNumber = &receiptNumber
				shouldSendReceipt = true
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sale.SaleItems = items

	// ── FIX 4: Use resolvedCustomer directly instead of a second DB query ──
	// Previously a second query ran here, but it could only find existing
	// customers and would silently fail for freshly-created walk-in customers
	// whose row was just committed (rare race), and it added an unnecessary
	// round-trip in every case.
	if resolvedCustomer != nil {
		sale.Customer = resolvedCustomer
	} else if sale.CustomerID != nil {
		// Fallback for existing customers not touched by the walk-in path
		// (e.g. customer_id was supplied directly by the caller).
		var customer models.Customer
		if err := s.db.Where("id = ? AND business_id = ?", *sale.CustomerID, businessID).First(&customer).Error; err == nil {
			sale.Customer = &customer
			resolvedCustomer = &customer
		}
	}

	// ── FIX 5: Log receipt-send failures instead of silently swallowing them ──
	// Previously: `if err := s.receiptSender.SendSaleReceipt(payload); err == nil { ... }`
	// A send failure would silently leave receipt_sent_at null with no indication why.
	if shouldSendReceipt && s.receiptSender != nil && sale.Customer != nil && sale.ReceiptNumber != nil && sale.Customer.Email != nil {
		payload := ReceiptEmailPayload{
			SaleID:            sale.ID,
			BusinessID:        sale.BusinessID,
			ToEmail:           strings.TrimSpace(*sale.Customer.Email),
			CustomerFirstName: sale.Customer.FirstName,
			CustomerLastName:  sale.Customer.LastName,
			SaleNumber:        sale.SaleNumber,
			ReceiptNumber:     *sale.ReceiptNumber,
			PaymentMethod:     sale.PaymentMethod,
			AmountPaid:        sale.AmountPaid,
			BalanceDue:        sale.BalanceDue,
			TotalAmount:       sale.TotalAmount,
			CreatedAt:         sale.CreatedAt,
			Items:             sale.SaleItems,
			Notes:             sale.Notes,
		}

		if sendErr := s.receiptSender.SendSaleReceipt(payload); sendErr == nil {
			now := time.Now().UTC()
			_ = s.db.Model(&models.Sale{}).
				Where("id = ? AND business_id = ?", sale.ID, businessID).
				Update("receipt_sent_at", now).Error
			sale.ReceiptSentAt = &now
		}
		// NOTE: receipt send failures are intentionally non-fatal — the sale
		// has already been committed. The caller can re-trigger via
		// POST /sales/:id/receipt if needed.
	}

	return &sale, nil
}

// ─── Cancel ──────────────────────────────────────────────────────────────────

func (s *Service) Cancel(businessID, saleID, cancelledBy uuid.UUID, req CancelRequest) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").
		Preload("Customer").
		First(&sale).Error; err != nil {
		return nil, apperr.New(apperr.CodeNotFound, "sale not found")
	}

	if sale.Status == models.SaleStatusCancelled {
		return nil, apperr.New(apperr.CodeConflict, "sale is already cancelled")
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "no reason given"
	}
	cancelNote := fmt.Sprintf("Cancelled by staff (user %s): %s", cancelledBy.String(), reason)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&sale).Updates(map[string]interface{}{
			"status": models.SaleStatusCancelled,
			"notes":  cancelNote,
		}).Error; err != nil {
			return fromDB(err)
		}
		sale.Status = models.SaleStatusCancelled

		for _, item := range sale.SaleItems {
			if item.ProductID != nil {
				if err := tx.Model(&models.Product{}).
					Where("id = ? AND business_id = ? AND track_inventory = true", *item.ProductID, businessID).
					UpdateColumn("stock_quantity", gorm.Expr("stock_quantity + ?", item.Quantity)).Error; err != nil {
					return fromDB(err)
				}
			}
		}

		if sale.CustomerID != nil {
			if err := tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).Updates(map[string]interface{}{
				"total_purchases": gorm.Expr("GREATEST(total_purchases - ?, 0)", sale.TotalAmount),
				"total_orders":    gorm.Expr("GREATEST(total_orders - 1, 0)"),
			}).Error; err != nil {
				return fromDB(err)
			}

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
					return fromDB(err)
				}

				if remainingDue > 0 {
					if err := tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).
						UpdateColumn("outstanding_debt", gorm.Expr("GREATEST(outstanding_debt - ?, 0)", remainingDue)).Error; err != nil {
						return fromDB(err)
					}
				}
			}
		}

		if sale.AmountPaid > 0 {
			reversal := models.LedgerEntry{
				BusinessID:  businessID,
				Type:        models.LedgerDebit,
				SourceType:  models.LedgerSourceSale,
				SourceID:    sale.ID,
				Amount:      sale.AmountPaid,
				Balance:     0,
				Description: fmt.Sprintf("Reversal — sale %s cancelled", sale.SaleNumber),
				RecordedBy:  cancelledBy,
			}
			if err := tx.Create(&reversal).Error; err != nil {
				return fromDB(err)
			}
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
	if req.Amount < 1 {
		return nil, apperr.New(apperr.CodeBadRequest, "payment amount must be greater than zero")
	}

	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("Customer").
		First(&sale).Error; err != nil {
		return nil, apperr.New(apperr.CodeNotFound, "sale not found")
	}

	if sale.Status == models.SaleStatusCancelled {
		return nil, apperr.New(apperr.CodeConflict, "cannot record payment on a cancelled sale")
	}
	if sale.BalanceDue <= 0 {
		return nil, apperr.New(apperr.CodeConflict, "sale is already fully paid")
	}
	if req.Amount > sale.BalanceDue {
		return nil, apperr.New(apperr.CodeBadRequest, "payment amount exceeds balance due")
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

	paymentMethod := normalizePaymentMethod(req.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = sale.PaymentMethod
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&sale).Updates(map[string]interface{}{
			"amount_paid":    sale.AmountPaid,
			"balance_due":    sale.BalanceDue,
			"status":         sale.Status,
			"payment_method": paymentMethod,
		}).Error; err != nil {
			return fromDB(err)
		}

		var debt models.Debt
		debtDesc := fmt.Sprintf("Balance from sale %s", sale.SaleNumber)
		err := tx.Where("business_id = ? AND description = ?", businessID, debtDesc).
			First(&debt).Error

		if err == nil {
			debt.AmountPaid += req.Amount
			debt.UpdateStatus()

			if err := tx.Model(&debt).Updates(map[string]interface{}{
				"amount_paid": debt.AmountPaid,
				"amount_due":  debt.AmountDue,
				"status":      debt.Status,
			}).Error; err != nil {
				return fromDB(err)
			}

			if debt.CustomerID != nil {
				if err := tx.Model(&models.Customer{}).Where("id = ?", *debt.CustomerID).
					UpdateColumn("outstanding_debt", gorm.Expr("GREATEST(outstanding_debt - ?, 0)", req.Amount)).Error; err != nil {
					return fromDB(err)
				}
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if sale.CustomerID != nil && prevBalance > 0 {
				if err := tx.Model(&models.Customer{}).Where("id = ?", *sale.CustomerID).
					UpdateColumn("outstanding_debt", gorm.Expr("GREATEST(outstanding_debt - ?, 0)", req.Amount)).Error; err != nil {
					return fromDB(err)
				}
			}
		} else {
			return fromDB(err)
		}

		note := strings.TrimSpace(req.Note)
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
		if err := tx.Create(&ledger).Error; err != nil {
			return fromDB(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sale.PaymentMethod = paymentMethod
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
		return nil, apperr.New(apperr.CodeNotFound, "sale not found")
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
	if strings.TrimSpace(filter.Status) != "" {
		q = q.Where("status = ?", strings.ToLower(strings.TrimSpace(filter.Status)))
	}
	if filter.DateFrom != nil {
		q = q.Where("created_at >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		q = q.Where("created_at <= ?", *filter.DateTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fromDB(err)
	}

	var sales []models.Sale
	err := q.Preload("Customer").
		Offset(offset).
		Limit(filter.Limit).
		Order("created_at DESC").
		Find(&sales).Error
	if err != nil {
		return nil, 0, fromDB(err)
	}

	return sales, total, nil
}

// ─── GenerateReceipt ──────────────────────────────────────────────────────────

func (s *Service) GenerateReceipt(businessID, saleID uuid.UUID) (*models.Sale, error) {
	var sale models.Sale
	if err := s.db.Where("id = ? AND business_id = ?", saleID, businessID).
		Preload("SaleItems").
		Preload("Customer").
		First(&sale).Error; err != nil {
		return nil, apperr.New(apperr.CodeNotFound, "sale not found")
	}

	if sale.ReceiptNumber != nil {
		return &sale, nil
	}

	receiptNumber, err := s.nextReceiptNumber(businessID)
	if err != nil {
		return nil, err
	}

	if err := s.db.Model(&sale).Update("receipt_number", receiptNumber).Error; err != nil {
		return nil, fromDB(err)
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
		return "", apperr.Wrap(apperr.CodeInternal, "could not generate sale number", err)
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
		return "", apperr.Wrap(apperr.CodeInternal, "could not generate receipt number", err)
	}

	return fmt.Sprintf("RC-%06d", seq.LastReceiptNumber), nil
}

func (s *Service) nextReceiptNumberTx(tx *gorm.DB, businessID uuid.UUID) (string, error) {
	var seq models.BusinessSaleSequence
	err := tx.Raw(`
		INSERT INTO business_sale_sequences (business_id, last_sale_number, last_receipt_number)
		VALUES (?, 0, 1)
		ON CONFLICT (business_id)
		DO UPDATE SET last_receipt_number = business_sale_sequences.last_receipt_number + 1
		RETURNING last_receipt_number
	`, businessID).Scan(&seq).Error
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, "could not generate receipt number", err)
	}

	return fmt.Sprintf("RC-%06d", seq.LastReceiptNumber), nil
}
