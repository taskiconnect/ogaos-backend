// internal/domain/models/expense.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	ExpenseTypeCOGS  = "cogs"
	ExpenseTypeOpex  = "opex"
	ExpenseTypeCapex = "capex"
	ExpenseTypeTax   = "tax_payment"

	ExpenseCategoryPurchaseOfGoods    = "purchase_of_goods"
	ExpenseCategoryRawMaterials       = "raw_materials"
	ExpenseCategoryPackaging          = "packaging_materials"
	ExpenseCategoryFreightInbound     = "freight_inbound"
	ExpenseCategoryRent               = "rent_and_rates"
	ExpenseCategorySalary             = "salaries_and_wages"
	ExpenseCategoryStaffBenefits      = "staff_benefits_allowances"
	ExpenseCategoryNSITF              = "nsitf"
	ExpenseCategoryPension            = "pension_employer"
	ExpenseCategoryUtilities          = "utilities"
	ExpenseCategoryTelecom            = "telephone_communication"
	ExpenseCategoryOfficeSupplies     = "office_supplies"
	ExpenseCategoryRepairs            = "repairs_maintenance"
	ExpenseCategoryFuelGenerator      = "fuel_generator"
	ExpenseCategoryTransport          = "transport_travel"
	ExpenseCategoryMarketing          = "marketing_advertising"
	ExpenseCategoryProfessionalFees   = "professional_fees"
	ExpenseCategoryBankCharges        = "bank_charges"
	ExpenseCategoryInsurance          = "insurance"
	ExpenseCategorySubscriptions      = "subscriptions_software"
	ExpenseCategorySecurityCharges    = "security_charges"
	ExpenseCategoryCleaningSanitation = "cleaning_sanitation"
	ExpenseCategoryMiscellaneous      = "miscellaneous"
	ExpenseCategoryEquipmentMachinery = "equipment_machinery"
	ExpenseCategoryFurnitureFixtures  = "furniture_fixtures"
	ExpenseCategoryVehicles           = "vehicles"
	ExpenseCategoryLandBuilding       = "land_building"
	ExpenseCategoryComputerTech       = "computer_technology"
	ExpenseCategoryVATRemittance      = "vat_remittance"
	ExpenseCategoryWHTRemittance      = "wht_remittance"
	ExpenseCategoryPAYE               = "paye"
	ExpenseCategoryCIT                = "company_income_tax"
	ExpenseCategoryBusinessLevy       = "business_premises_levy"
)

type Expense struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	StoreID         *uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	ExpenseType     string     `gorm:"size:20;not null;default:opex" json:"expense_type"`
	Category        string     `gorm:"size:60;not null" json:"category"`
	Description     string     `gorm:"type:text;not null" json:"description"`
	Amount          int64      `gorm:"not null" json:"amount"`
	VATInclusive    bool       `gorm:"default:false" json:"vat_inclusive"`
	VATRate         float64    `gorm:"default:0" json:"vat_rate"`
	VATAmount       int64      `gorm:"default:0" json:"vat_amount"`
	IsTaxDeductible bool       `gorm:"default:true" json:"is_tax_deductible"`
	AssetLifeYears  *int       `json:"asset_life_years"`
	AssetStartDate  *time.Time `json:"asset_start_date"`
	ReceiptURL      *string    `gorm:"size:500" json:"receipt_url"`
	ExpenseDate     time.Time  `gorm:"not null;index" json:"expense_date"`
	RecordedBy      uuid.UUID  `gorm:"type:uuid;not null" json:"recorded_by"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	Business        Business   `gorm:"foreignKey:BusinessID" json:"-"`
	Store           *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
}

func (e *Expense) MonthlyDepreciation() int64 {
	if e.ExpenseType != ExpenseTypeCapex || e.AssetLifeYears == nil || *e.AssetLifeYears <= 0 {
		return 0
	}
	return e.Amount / int64(*e.AssetLifeYears) / 12
}

func (e *Expense) CalculateInputVAT() {
	if !e.VATInclusive || e.VATRate == 0 {
		e.VATAmount = 0
		return
	}
	base := float64(e.Amount) / (1 + e.VATRate/100)
	e.VATAmount = int64(float64(e.Amount) - base)
}
