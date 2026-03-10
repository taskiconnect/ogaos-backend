// internal/service/invoice/service.go
package invoice

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/cursor"
	"ogaos-backend/internal/pkg/email"
)

type Service struct {
	db          *gorm.DB
	frontendURL string
}

func NewService(db *gorm.DB, frontendURL string) *Service {
	return &Service{db: db, frontendURL: frontendURL}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type InvoiceItemInput struct {
	ProductID    *uuid.UUID `json:"product_id"`
	Description  string     `json:"description" binding:"required"`
	UnitPrice    int64      `json:"unit_price" binding:"required,min=1"`
	Quantity     int        `json:"quantity" binding:"required,min=1"`
	Discount     int64      `json:"discount"`
	VATInclusive bool       `json:"vat_inclusive"`
}

type CreateRequest struct {
	StoreID        *uuid.UUID         `json:"store_id"`
	CustomerID     *uuid.UUID         `json:"customer_id"`
	DueDate        time.Time          `json:"due_date" binding:"required"`
	Items          []InvoiceItemInput `json:"items" binding:"required,min=1"`
	DiscountAmount int64              `json:"discount_amount"`
	VATRate        float64            `json:"vat_rate"`
	VATInclusive   bool               `json:"vat_inclusive"`
	WHTRate        float64            `json:"wht_rate"`
	PaymentTerms   *string            `json:"payment_terms"`
	Notes          *string            `json:"notes"`
}

type ListFilter struct {
	Status     string
	CustomerID *uuid.UUID
	DateFrom   *time.Time
	DateTo     *time.Time
	Cursor     string
	Limit      int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, createdBy uuid.UUID, req CreateRequest) (*models.Invoice, error) {
	invoiceNumber, err := s.nextInvoiceNumber(businessID)
	if err != nil {
		return nil, err
	}

	var subTotal int64
	var items []models.InvoiceItem
	for _, item := range req.Items {
		lineTotal := (item.UnitPrice * int64(item.Quantity)) - item.Discount
		items = append(items, models.InvoiceItem{
			ProductID:    item.ProductID,
			Description:  item.Description,
			UnitPrice:    item.UnitPrice,
			Quantity:     item.Quantity,
			Discount:     item.Discount,
			TotalPrice:   lineTotal,
			VATInclusive: item.VATInclusive,
		})
		subTotal += lineTotal
	}

	inv := models.Invoice{
		BusinessID:     businessID,
		StoreID:        req.StoreID,
		CustomerID:     req.CustomerID,
		CreatedBy:      createdBy,
		InvoiceNumber:  invoiceNumber,
		IssueDate:      time.Now(),
		DueDate:        req.DueDate,
		SubTotal:       subTotal,
		DiscountAmount: req.DiscountAmount,
		VATRate:        req.VATRate,
		VATInclusive:   req.VATInclusive,
		WHTRate:        req.WHTRate,
		PaymentTerms:   req.PaymentTerms,
		Notes:          req.Notes,
		Status:         models.InvoiceStatusDraft,
	}
	inv.CalculateVAT()
	inv.CalculateWHT()
	inv.CalculateTotal()

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&inv).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].InvoiceID = inv.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}
	inv.InvoiceItems = items
	return &inv, nil
}

func (s *Service) Get(businessID, invoiceID uuid.UUID) (*models.Invoice, error) {
	var inv models.Invoice
	err := s.db.Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("InvoiceItems").
		Preload("Customer").
		First(&inv).Error
	if err != nil {
		return nil, errors.New("invoice not found")
	}
	return &inv, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Invoice, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	q := s.db.Model(&models.Invoice{}).Where("business_id = ?", businessID)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.CustomerID != nil {
		q = q.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.DateFrom != nil {
		q = q.Where("issue_date >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		q = q.Where("issue_date <= ?", *filter.DateTo)
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cur, id)
	}

	var invoices []models.Invoice
	if err := q.Preload("Customer").Order("created_at DESC, id DESC").Limit(filter.Limit + 1).Find(&invoices).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(invoices) > filter.Limit {
		last := invoices[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		invoices = invoices[:filter.Limit]
	}
	return invoices, nextCursor, nil
}

// Send marks an invoice as sent and emails the customer.
func (s *Service) Send(businessID, invoiceID uuid.UUID) (*models.Invoice, error) {
	var inv models.Invoice
	if err := s.db.Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("Customer").Preload("Business").First(&inv).Error; err != nil {
		return nil, errors.New("invoice not found")
	}
	if inv.Status == models.InvoiceStatusCancelled {
		return nil, errors.New("cannot send a cancelled invoice")
	}

	now := time.Now()
	s.db.Model(&inv).Updates(map[string]interface{}{
		"status":  models.InvoiceStatusSent,
		"sent_at": now,
	})
	inv.Status = models.InvoiceStatusSent
	inv.SentAt = &now

	// Email the customer if we have their address
	if inv.Customer != nil && inv.Customer.Email != nil {
		viewURL := fmt.Sprintf("%s/invoices/%s", s.frontendURL, inv.ID)
		email.SendInvoice(
			*inv.Customer.Email,
			inv.Customer.FirstName+" "+inv.Customer.LastName,
			inv.Business.Name,
			inv.InvoiceNumber,
			viewURL,
		)
	}

	return &inv, nil
}

// RecordPayment records a payment against an invoice, updates status.
func (s *Service) RecordPayment(businessID, invoiceID uuid.UUID, amountPaid int64) (*models.Invoice, error) {
	var inv models.Invoice
	if err := s.db.Where("id = ? AND business_id = ?", invoiceID, businessID).First(&inv).Error; err != nil {
		return nil, errors.New("invoice not found")
	}
	if inv.Status == models.InvoiceStatusCancelled {
		return nil, errors.New("cannot record payment on a cancelled invoice")
	}
	if inv.Status == models.InvoiceStatusPaid {
		return nil, errors.New("invoice is already fully paid")
	}

	newAmountPaid := inv.AmountPaid + amountPaid
	if newAmountPaid > inv.TotalAmount {
		return nil, errors.New("payment exceeds invoice total")
	}

	updates := map[string]interface{}{
		"amount_paid": newAmountPaid,
		"balance_due": inv.TotalAmount - newAmountPaid,
	}
	if newAmountPaid >= inv.TotalAmount {
		now := time.Now()
		updates["status"] = models.InvoiceStatusPaid
		updates["paid_at"] = now
	} else {
		updates["status"] = models.InvoiceStatusPartial
	}

	s.db.Model(&inv).Updates(updates)
	return &inv, nil
}

// Cancel cancels a draft or sent invoice.
func (s *Service) Cancel(businessID, invoiceID uuid.UUID) error {
	result := s.db.Model(&models.Invoice{}).
		Where("id = ? AND business_id = ? AND status IN ?", invoiceID, businessID, []string{
			models.InvoiceStatusDraft, models.InvoiceStatusSent, models.InvoiceStatusOverdue,
		}).
		Update("status", models.InvoiceStatusCancelled)
	if result.RowsAffected == 0 {
		return errors.New("invoice not found or cannot be cancelled in its current state")
	}
	return result.Error
}

// MarkOverdue updates all sent invoices past their due date to overdue.
// Called by a scheduled job daily.
func (s *Service) MarkOverdue() error {
	return s.db.Model(&models.Invoice{}).
		Where("status = ? AND due_date < ?", models.InvoiceStatusSent, time.Now()).
		Update("status", models.InvoiceStatusOverdue).Error
}

// ─── Number generation ────────────────────────────────────────────────────────

func (s *Service) nextInvoiceNumber(businessID uuid.UUID) (string, error) {
	prefix := fmt.Sprintf("INV-%s-", time.Now().Format("200601"))
	var last models.Invoice
	err := s.db.Where("business_id = ? AND invoice_number LIKE ?", businessID, prefix+"%").
		Order("invoice_number DESC").First(&last).Error

	seq := 1
	if err == nil {
		fmt.Sscanf(last.InvoiceNumber[len(prefix):], "%d", &seq)
		seq++
	}
	return fmt.Sprintf("%s%04d", prefix, seq), nil
}
