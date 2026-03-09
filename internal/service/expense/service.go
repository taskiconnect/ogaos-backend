// internal/service/expense/service.go
package expense

import (
	"errors"
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

type CreateRequest struct {
	StoreID         *uuid.UUID `json:"store_id"`
	ExpenseType     string     `json:"expense_type" binding:"required"` // cogs | opex | capex | tax_payment
	Category        string     `json:"category" binding:"required"`
	Description     string     `json:"description" binding:"required"`
	Amount          int64      `json:"amount" binding:"required,min=1"`
	VATInclusive    bool       `json:"vat_inclusive"`
	VATRate         float64    `json:"vat_rate"`
	IsTaxDeductible bool       `json:"is_tax_deductible"`
	// CapEx fields
	AssetLifeYears *int       `json:"asset_life_years"`
	AssetStartDate *time.Time `json:"asset_start_date"`
	ReceiptURL     *string    `json:"receipt_url"`
	ExpenseDate    *time.Time `json:"expense_date"`
}

type UpdateRequest struct {
	Category        *string  `json:"category"`
	Description     *string  `json:"description"`
	Amount          *int64   `json:"amount"`
	VATInclusive    *bool    `json:"vat_inclusive"`
	VATRate         *float64 `json:"vat_rate"`
	IsTaxDeductible *bool    `json:"is_tax_deductible"`
	ReceiptURL      *string  `json:"receipt_url"`
}

type ListFilter struct {
	StoreID     *uuid.UUID
	ExpenseType string
	Category    string
	DateFrom    *time.Time
	DateTo      *time.Time
	Page        int
	Limit       int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID, recordedBy uuid.UUID, req CreateRequest) (*models.Expense, error) {
	validTypes := map[string]bool{
		models.ExpenseTypeCOGS: true, models.ExpenseTypeOpex: true,
		models.ExpenseTypeCapex: true, models.ExpenseTypeTax: true,
	}
	if !validTypes[req.ExpenseType] {
		return nil, errors.New("expense_type must be cogs, opex, capex, or tax_payment")
	}
	if req.ExpenseType == models.ExpenseTypeCapex && (req.AssetLifeYears == nil || *req.AssetLifeYears == 0) {
		return nil, errors.New("asset_life_years is required for capex expenses")
	}

	expDate := time.Now()
	if req.ExpenseDate != nil {
		expDate = *req.ExpenseDate
	}

	e := models.Expense{
		BusinessID:      businessID,
		StoreID:         req.StoreID,
		ExpenseType:     req.ExpenseType,
		Category:        req.Category,
		Description:     req.Description,
		Amount:          req.Amount,
		VATInclusive:    req.VATInclusive,
		VATRate:         req.VATRate,
		IsTaxDeductible: req.IsTaxDeductible,
		AssetLifeYears:  req.AssetLifeYears,
		AssetStartDate:  req.AssetStartDate,
		ReceiptURL:      req.ReceiptURL,
		RecordedBy:      recordedBy,
		ExpenseDate:     expDate,
	}
	e.CalculateInputVAT()

	return &e, s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		// Ledger debit entry
		ledger := models.LedgerEntry{
			BusinessID:  businessID,
			Type:        models.LedgerDebit,
			SourceType:  models.LedgerSourceExpense,
			SourceID:    e.ID,
			Amount:      e.Amount,
			Balance:     0, // running balance computed by ledger query
			Description: e.Description,
			RecordedBy:  recordedBy,
		}
		return tx.Create(&ledger).Error
	})
}

func (s *Service) Get(businessID, expenseID uuid.UUID) (*models.Expense, error) {
	var e models.Expense
	if err := s.db.Where("id = ? AND business_id = ?", expenseID, businessID).First(&e).Error; err != nil {
		return nil, errors.New("expense not found")
	}
	return &e, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Expense, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit

	q := s.db.Model(&models.Expense{}).Where("business_id = ?", businessID)
	if filter.StoreID != nil {
		q = q.Where("store_id = ?", *filter.StoreID)
	}
	if filter.ExpenseType != "" {
		q = q.Where("expense_type = ?", filter.ExpenseType)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.DateFrom != nil {
		q = q.Where("expense_date >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		q = q.Where("expense_date <= ?", *filter.DateTo)
	}

	var total int64
	q.Count(&total)

	var expenses []models.Expense
	err := q.Offset(offset).Limit(filter.Limit).Order("expense_date DESC").Find(&expenses).Error
	return expenses, total, err
}

func (s *Service) Update(businessID, expenseID uuid.UUID, req UpdateRequest) (*models.Expense, error) {
	var e models.Expense
	if err := s.db.Where("id = ? AND business_id = ?", expenseID, businessID).First(&e).Error; err != nil {
		return nil, errors.New("expense not found")
	}
	updates := map[string]interface{}{}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Amount != nil {
		updates["amount"] = *req.Amount
	}
	if req.VATInclusive != nil {
		updates["vat_inclusive"] = *req.VATInclusive
	}
	if req.VATRate != nil {
		updates["vat_rate"] = *req.VATRate
	}
	if req.IsTaxDeductible != nil {
		updates["is_tax_deductible"] = *req.IsTaxDeductible
	}
	if req.ReceiptURL != nil {
		updates["receipt_url"] = *req.ReceiptURL
	}
	if err := s.db.Model(&e).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Service) Delete(businessID, expenseID uuid.UUID) error {
	result := s.db.Where("id = ? AND business_id = ?", expenseID, businessID).Delete(&models.Expense{})
	if result.RowsAffected == 0 {
		return errors.New("expense not found")
	}
	return result.Error
}

// MonthlySummary returns totals by expense type for a given year/month.
// Used to populate the tax_summary service.
type MonthlySummaryResult struct {
	TotalCOGS         int64
	TotalOpex         int64
	TotalCapex        int64
	TotalSalaries     int64
	TotalRent         int64
	TotalDepreciation int64 // computed from capex assets
	VATOnExpenses     int64
}

func (s *Service) MonthlySummary(businessID uuid.UUID, year, month int) (MonthlySummaryResult, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	type row struct {
		ExpenseType string
		Category    string
		Total       int64
		TotalVAT    int64
	}
	var rows []row
	s.db.Model(&models.Expense{}).
		Select("expense_type, category, SUM(amount) as total, SUM(vat_amount) as total_vat").
		Where("business_id = ? AND expense_date >= ? AND expense_date < ?", businessID, start, end).
		Group("expense_type, category").
		Scan(&rows)

	var result MonthlySummaryResult
	for _, r := range rows {
		switch r.ExpenseType {
		case models.ExpenseTypeCOGS:
			result.TotalCOGS += r.Total
		case models.ExpenseTypeOpex:
			result.TotalOpex += r.Total
			if r.Category == models.ExpenseCategorySalary {
				result.TotalSalaries += r.Total
			}
			if r.Category == models.ExpenseCategoryRent {
				result.TotalRent += r.Total
			}
		case models.ExpenseTypeCapex:
			result.TotalCapex += r.Total
		}
		result.VATOnExpenses += r.TotalVAT
	}

	// Compute monthly depreciation from all active capex assets
	var capexAssets []models.Expense
	s.db.Where("business_id = ? AND expense_type = ? AND asset_life_years > 0", businessID, models.ExpenseTypeCapex).
		Find(&capexAssets)
	for _, asset := range capexAssets {
		result.TotalDepreciation += asset.MonthlyDepreciation()
	}

	return result, nil
}
