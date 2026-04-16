package invoice

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/cursor"
	emailpkg "ogaos-backend/internal/pkg/email"
	pdfpkg "ogaos-backend/internal/pkg/pdf"
)

type Service struct {
	db          *gorm.DB
	frontendURL string
}

func NewService(db *gorm.DB, frontendURL string) *Service {
	return &Service{
		db:          db,
		frontendURL: strings.TrimRight(frontendURL, "/"),
	}
}

type DateOnly struct{ time.Time }

func (d *DateOnly) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)

	if s == "null" || s == "" {
		return nil
	}

	if t, err := time.Parse(time.RFC3339, s); err == nil {
		d.Time = t
		return nil
	}

	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fmt.Errorf("date must be YYYY-MM-DD or RFC-3339, got %q", s)
	}
	d.Time = t
	return nil
}

type InvoiceItemInput struct {
	ProductID    *uuid.UUID `json:"product_id"`
	Description  string     `json:"description" binding:"required"`
	ProductSKU   *string    `json:"product_sku"`
	UnitPrice    int64      `json:"unit_price" binding:"required,min=1"`
	Quantity     int        `json:"quantity" binding:"required,min=1"`
	Discount     int64      `json:"discount"`
	VATInclusive bool       `json:"vat_inclusive"`
}

type CreateRequest struct {
	StoreID        *uuid.UUID         `json:"store_id"`
	CustomerID     *uuid.UUID         `json:"customer_id"`
	IssueDate      *DateOnly          `json:"issue_date"`
	DueDate        DateOnly           `json:"due_date" binding:"required"`
	Items          []InvoiceItemInput `json:"items" binding:"required,min=1"`
	DiscountAmount int64              `json:"discount_amount"`
	VATRate        float64            `json:"vat_rate"`
	VATInclusive   bool               `json:"vat_inclusive"`
	WHTRate        float64            `json:"wht_rate"`
	PaymentTerms   *string            `json:"payment_terms"`
	Notes          *string            `json:"notes"`
}

type UpdateRequest struct {
	StoreID        *uuid.UUID         `json:"store_id"`
	CustomerID     *uuid.UUID         `json:"customer_id"`
	IssueDate      *DateOnly          `json:"issue_date"`
	DueDate        *DateOnly          `json:"due_date"`
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

var ErrNotFound = errors.New("invoice not found")
var ErrInvoiceLocked = errors.New("invoice can no longer be edited directly; create a revision instead")

func (s *Service) Create(businessID, createdBy uuid.UUID, req CreateRequest) (*models.Invoice, error) {
	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}

	items, subTotal := buildInvoiceItems(req.Items)

	issueDate := time.Now()
	if req.IssueDate != nil && !req.IssueDate.IsZero() {
		issueDate = req.IssueDate.Time
	}

	inv := models.Invoice{
		BusinessID:     businessID,
		StoreID:        req.StoreID,
		CustomerID:     req.CustomerID,
		CreatedBy:      createdBy,
		PublicToken:    token,
		RevisionNumber: 1,
		IssueDate:      issueDate,
		DueDate:        req.DueDate.Time,
		SubTotal:       subTotal,
		DiscountAmount: req.DiscountAmount,
		VATRate:        req.VATRate,
		VATInclusive:   req.VATInclusive,
		WHTRate:        req.WHTRate,
		PaymentTerms:   req.PaymentTerms,
		Notes:          req.Notes,
		Status:         models.InvoiceStatusDraft,
	}
	inv.CalculateTotal()

	err = s.db.Transaction(func(tx *gorm.DB) error {
		invoiceNumber, err := s.nextInvoiceNumberTx(tx, businessID, issueDate)
		if err != nil {
			return err
		}
		inv.InvoiceNumber = invoiceNumber

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

func (s *Service) Update(businessID, invoiceID uuid.UUID, req UpdateRequest) (*models.Invoice, error) {
	var inv models.Invoice
	if err := s.db.
		Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("InvoiceItems").
		First(&inv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if !inv.IsEditable() {
		return nil, ErrInvoiceLocked
	}

	items, subTotal := buildInvoiceItems(req.Items)

	if req.StoreID != nil || inv.StoreID != nil {
		inv.StoreID = req.StoreID
	}
	if req.CustomerID != nil || inv.CustomerID != nil {
		inv.CustomerID = req.CustomerID
	}
	if req.IssueDate != nil && !req.IssueDate.IsZero() {
		inv.IssueDate = req.IssueDate.Time
	}
	if req.DueDate != nil && !req.DueDate.IsZero() {
		inv.DueDate = req.DueDate.Time
	}

	inv.SubTotal = subTotal
	inv.DiscountAmount = req.DiscountAmount
	inv.VATRate = req.VATRate
	inv.VATInclusive = req.VATInclusive
	inv.WHTRate = req.WHTRate
	inv.PaymentTerms = req.PaymentTerms
	inv.Notes = req.Notes
	inv.CalculateTotal()

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Invoice{}).
			Where("id = ? AND business_id = ?", inv.ID, businessID).
			Updates(map[string]interface{}{
				"store_id":        inv.StoreID,
				"customer_id":     inv.CustomerID,
				"issue_date":      inv.IssueDate,
				"due_date":        inv.DueDate,
				"sub_total":       inv.SubTotal,
				"discount_amount": inv.DiscountAmount,
				"vat_rate":        inv.VATRate,
				"vat_inclusive":   inv.VATInclusive,
				"vat_amount":      inv.VATAmount,
				"wht_rate":        inv.WHTRate,
				"wht_amount":      inv.WHTAmount,
				"total_amount":    inv.TotalAmount,
				"balance_due":     inv.BalanceDue,
				"payment_terms":   inv.PaymentTerms,
				"notes":           inv.Notes,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("invoice_id = ?", inv.ID).Delete(&models.InvoiceItem{}).Error; err != nil {
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

func (s *Service) Revise(businessID, createdBy, invoiceID uuid.UUID) (*models.Invoice, error) {
	var original models.Invoice
	if err := s.db.
		Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("InvoiceItems").
		First(&original).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if original.Status == models.InvoiceStatusCancelled || original.Status == models.InvoiceStatusSuperseded {
		return nil, errors.New("invoice cannot be revised in its current state")
	}

	if original.SupersededByInvoiceID != nil {
		return nil, errors.New("invoice has already been revised")
	}

	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}

	newInv := models.Invoice{
		BusinessID:           original.BusinessID,
		StoreID:              original.StoreID,
		CustomerID:           original.CustomerID,
		CreatedBy:            createdBy,
		PublicToken:          token,
		RevisionNumber:       original.RevisionNumber + 1,
		RevisedFromInvoiceID: &original.ID,
		IssueDate:            time.Now(),
		DueDate:              original.DueDate,
		SubTotal:             original.SubTotal,
		DiscountAmount:       original.DiscountAmount,
		VATRate:              original.VATRate,
		VATInclusive:         original.VATInclusive,
		VATAmount:            original.VATAmount,
		WHTRate:              original.WHTRate,
		WHTAmount:            original.WHTAmount,
		TotalAmount:          original.TotalAmount,
		AmountPaid:           0,
		BalanceDue:           original.TotalAmount,
		Currency:             original.Currency,
		Status:               models.InvoiceStatusDraft,
		PaymentTerms:         original.PaymentTerms,
		Notes:                original.Notes,
	}

	items := make([]models.InvoiceItem, 0, len(original.InvoiceItems))
	for _, item := range original.InvoiceItems {
		items = append(items, models.InvoiceItem{
			ProductID:    item.ProductID,
			Description:  item.Description,
			ProductSKU:   item.ProductSKU,
			UnitPrice:    item.UnitPrice,
			Quantity:     item.Quantity,
			Discount:     item.Discount,
			TotalPrice:   item.TotalPrice,
			VATInclusive: item.VATInclusive,
		})
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		newNumber, err := s.nextInvoiceNumberTx(tx, businessID, newInv.IssueDate)
		if err != nil {
			return err
		}
		newInv.InvoiceNumber = newNumber

		if err := tx.Create(&newInv).Error; err != nil {
			return err
		}

		for i := range items {
			items[i].InvoiceID = newInv.ID
		}

		if err := tx.Create(&items).Error; err != nil {
			return err
		}

		return tx.Model(&models.Invoice{}).
			Where("id = ? AND business_id = ?", original.ID, businessID).
			Updates(map[string]interface{}{
				"status":                   models.InvoiceStatusSuperseded,
				"superseded_by_invoice_id": newInv.ID,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	newInv.InvoiceItems = items
	return &newInv, nil
}

func (s *Service) Get(businessID, invoiceID uuid.UUID) (*models.Invoice, error) {
	var inv models.Invoice
	err := s.db.
		Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("InvoiceItems").
		Preload("Customer").
		First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Service) GetPublicByToken(token string) (*models.Invoice, error) {
	var inv models.Invoice
	err := s.db.
		Where("public_token = ?", token).
		Preload("InvoiceItems").
		Preload("Customer").
		Preload("Business").
		First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	switch inv.Status {
	case models.InvoiceStatusCancelled, models.InvoiceStatusSuperseded:
		return nil, ErrNotFound
	}

	return &inv, nil
}

func (s *Service) BuildPDFForInvoice(inv *models.Invoice) ([]byte, string, error) {
	businessName := "Business"
	if inv.Business.Name != "" {
		businessName = inv.Business.Name
	}

	pdf, err := pdfpkg.BuildInvoicePDF(inv, businessName)
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("%s-rev-%d.pdf", inv.InvoiceNumber, inv.RevisionNumber)
	filename = strings.ReplaceAll(filename, " ", "-")
	return pdf, filename, nil
}

func (s *Service) BuildPDFForProtectedInvoice(businessID, invoiceID uuid.UUID) ([]byte, string, error) {
	var inv models.Invoice
	err := s.db.
		Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("InvoiceItems").
		Preload("Customer").
		Preload("Business").
		First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return s.BuildPDFForInvoice(&inv)
}

func (s *Service) BuildPDFForPublicToken(token string) ([]byte, string, error) {
	inv, err := s.GetPublicByToken(token)
	if err != nil {
		return nil, "", err
	}
	return s.BuildPDFForInvoice(inv)
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

func (s *Service) Send(businessID, invoiceID uuid.UUID) (*models.Invoice, error) {
	var inv models.Invoice
	if err := s.db.
		Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("Customer").
		Preload("Business").
		Preload("InvoiceItems").
		First(&inv).Error; err != nil {
		return nil, ErrNotFound
	}

	if inv.Status == models.InvoiceStatusCancelled || inv.Status == models.InvoiceStatusSuperseded {
		return nil, errors.New("cannot send this invoice in its current state")
	}

	now := time.Now()
	if err := s.db.Model(&inv).Updates(map[string]interface{}{
		"status":  models.InvoiceStatusSent,
		"sent_at": now,
	}).Error; err != nil {
		return nil, err
	}
	inv.Status = models.InvoiceStatusSent
	inv.SentAt = &now

	if inv.Customer != nil && inv.Customer.Email != nil {
		viewURL := fmt.Sprintf("%s/public/invoices/%s", s.frontendURL, inv.PublicToken)
		pdfBytes, filename, err := s.BuildPDFForInvoice(&inv)
		if err != nil {
			return nil, err
		}

		customerName := strings.TrimSpace(inv.Customer.FirstName + " " + inv.Customer.LastName)
		if customerName == "" {
			customerName = "Customer"
		}

		go emailpkg.SendInvoice(
			*inv.Customer.Email,
			customerName,
			inv.Business.Name,
			inv.InvoiceNumber,
			viewURL,
			pdfBytes,
			filename,
		)
	}

	return &inv, nil
}

func (s *Service) RecordPayment(businessID, invoiceID uuid.UUID, amountPaid int64) (*models.Invoice, error) {
	var inv models.Invoice
	if err := s.db.Where("id = ? AND business_id = ?", invoiceID, businessID).First(&inv).Error; err != nil {
		return nil, ErrNotFound
	}

	if inv.Status == models.InvoiceStatusCancelled || inv.Status == models.InvoiceStatusSuperseded {
		return nil, errors.New("cannot record payment on this invoice")
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

	if err := s.db.Model(&inv).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.Get(businessID, invoiceID)
}

func (s *Service) Cancel(businessID, invoiceID uuid.UUID) error {
	result := s.db.Model(&models.Invoice{}).
		Where("id = ? AND business_id = ? AND status IN ?", invoiceID, businessID, []string{
			models.InvoiceStatusDraft,
			models.InvoiceStatusSent,
			models.InvoiceStatusOverdue,
		}).
		Update("status", models.InvoiceStatusCancelled)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("invoice not found or cannot be cancelled in its current state")
	}
	return nil
}

func (s *Service) MarkOverdue() error {
	return s.db.Model(&models.Invoice{}).
		Where("status = ? AND due_date < ?", models.InvoiceStatusSent, time.Now()).
		Update("status", models.InvoiceStatusOverdue).Error
}

func (s *Service) nextInvoiceNumberTx(tx *gorm.DB, businessID uuid.UUID, issueDate time.Time) (string, error) {
	period := issueDate.Format("200601")

	var seq models.BusinessInvoiceSequence
	err := tx.Raw(`
		INSERT INTO business_invoice_sequences (business_id, period, last_invoice_number)
		VALUES (?, ?, 1)
		ON CONFLICT (business_id, period)
		DO UPDATE SET last_invoice_number = business_invoice_sequences.last_invoice_number + 1
		RETURNING business_id, period, last_invoice_number
	`, businessID, period).Scan(&seq).Error
	if err != nil {
		return "", fmt.Errorf("could not generate invoice number: %w", err)
	}

	return fmt.Sprintf("INV-%s-%04d", period, seq.LastInvoiceNumber), nil
}

func buildInvoiceItems(inputs []InvoiceItemInput) ([]models.InvoiceItem, int64) {
	items := make([]models.InvoiceItem, 0, len(inputs))
	var subTotal int64

	for _, item := range inputs {
		lineTotal := (item.UnitPrice * int64(item.Quantity)) - item.Discount
		if lineTotal < 0 {
			lineTotal = 0
		}

		items = append(items, models.InvoiceItem{
			ProductID:    item.ProductID,
			Description:  item.Description,
			ProductSKU:   item.ProductSKU,
			UnitPrice:    item.UnitPrice,
			Quantity:     item.Quantity,
			Discount:     item.Discount,
			TotalPrice:   lineTotal,
			VATInclusive: item.VATInclusive,
		})
		subTotal += lineTotal
	}

	return items, subTotal
}

func generatePublicToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate public token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
