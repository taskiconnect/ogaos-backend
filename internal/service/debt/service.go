package debt

import (
	"errors"
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
	Direction     string     `json:"direction" binding:"required"` // receivable | payable
	CustomerID    *uuid.UUID `json:"customer_id"`
	SupplierName  *string    `json:"supplier_name"`
	SupplierPhone *string    `json:"supplier_phone"`
	Description   string     `json:"description" binding:"required"`
	TotalAmount   int64      `json:"total_amount" binding:"required,min=1"`
	DueDate       *time.Time `json:"due_date"`
	Notes         *string    `json:"notes"`
}

type RecordPaymentRequest struct {
	Amount int64  `json:"amount" binding:"required,min=1"`
	Notes  string `json:"notes"`
}

type ListFilter struct {
	Direction  string
	Status     string
	CustomerID *uuid.UUID
	Overdue    bool
	Cursor     string
	Limit      int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Debt, error) {
	if req.Direction != models.DebtDirectionReceivable && req.Direction != models.DebtDirectionPayable {
		return nil, errors.New("direction must be 'receivable' or 'payable'")
	}

	if req.Direction == models.DebtDirectionReceivable && req.CustomerID == nil {
		return nil, errors.New("customer_id is required for receivable debts")
	}

	if req.Direction == models.DebtDirectionPayable && req.SupplierName == nil {
		return nil, errors.New("supplier_name is required for payable debts")
	}

	d := models.Debt{
		BusinessID:    businessID,
		Direction:     req.Direction,
		CustomerID:    req.CustomerID,
		SupplierName:  req.SupplierName,
		SupplierPhone: req.SupplierPhone,
		Description:   req.Description,
		TotalAmount:   req.TotalAmount,
		AmountPaid:    0,
		AmountDue:     req.TotalAmount,
		DueDate:       req.DueDate,
		Notes:         req.Notes,
		RecordedBy:    recordedBy,
		Status:        models.DebtStatusOutstanding,
	}

	d.UpdateStatus()

	if err := s.db.Create(&d).Error; err != nil {
		return nil, err
	}

	// Update customer's outstanding debt counter
	if req.CustomerID != nil && req.Direction == models.DebtDirectionReceivable {
		s.db.Model(&models.Customer{}).
			Where("id = ?", *req.CustomerID).
			UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt + ?", req.TotalAmount))
	}

	return &d, nil
}

func (s *Service) Get(businessID, debtID uuid.UUID) (*models.Debt, error) {
	var d models.Debt
	if err := s.db.Where("id = ? AND business_id = ?", debtID, businessID).
		Preload("Customer").
		First(&d).Error; err != nil {
		return nil, errors.New("debt not found")
	}
	return &d, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Debt, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	q := s.db.Model(&models.Debt{}).Where("business_id = ?", businessID)

	if filter.Direction != "" {
		q = q.Where("direction = ?", filter.Direction)
	}

	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}

	if filter.CustomerID != nil {
		q = q.Where("customer_id = ?", *filter.CustomerID)
	}

	if filter.Overdue {
		q = q.Where(
			"due_date < ? AND status NOT IN ?",
			time.Now().UTC(),
			[]string{models.DebtStatusSettled},
		)
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cur, id)
	}

	var debts []models.Debt
	if err := q.Preload("Customer").
		Order("created_at DESC, id DESC").
		Limit(filter.Limit + 1).
		Find(&debts).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(debts) > filter.Limit {
		last := debts[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		debts = debts[:filter.Limit]
	}

	return debts, nextCursor, nil
}

// RecordPayment applies a payment to a debt, recalculates status.
func (s *Service) RecordPayment(businessID, debtID uuid.UUID, req RecordPaymentRequest) (*models.Debt, error) {
	var d models.Debt
	if err := s.db.Where("id = ? AND business_id = ?", debtID, businessID).First(&d).Error; err != nil {
		return nil, errors.New("debt not found")
	}

	if d.Status == models.DebtStatusSettled {
		return nil, errors.New("debt is already settled")
	}

	if req.Amount > d.AmountDue {
		return nil, errors.New("payment exceeds outstanding amount")
	}

	d.AmountPaid += req.Amount
	d.UpdateStatus()

	if err := s.db.Model(&d).Updates(map[string]interface{}{
		"amount_paid": d.AmountPaid,
		"amount_due":  d.AmountDue,
		"status":      d.Status,
	}).Error; err != nil {
		return nil, err
	}

	// Update customer outstanding debt counter for receivables
	if d.CustomerID != nil && d.Direction == models.DebtDirectionReceivable {
		s.db.Model(&models.Customer{}).
			Where("id = ?", *d.CustomerID).
			UpdateColumn("outstanding_debt", gorm.Expr("outstanding_debt - ?", req.Amount))
	}

	return &d, nil
}

// MarkOverdue updates all debts past their due date to overdue status.
// Called by a scheduled job daily.
func (s *Service) MarkOverdue() error {
	return s.db.Model(&models.Debt{}).
		Where("due_date < ? AND status NOT IN ?", time.Now().UTC(), []string{models.DebtStatusSettled}).
		Update("status", models.DebtStatusOverdue).Error
}
