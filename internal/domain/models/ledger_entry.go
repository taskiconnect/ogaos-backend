// internal/domain/models/ledger_entry.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	LedgerCredit = "credit" // money coming in
	LedgerDebit  = "debit"  // money going out

	LedgerSourceSale        = "sale"
	LedgerSourceExpense     = "expense"
	LedgerSourceDebtPayment = "debt_payment"
	LedgerSourceDigitalSale = "digital_sale"
	LedgerSourceRefund      = "refund"
	LedgerSourceManual      = "manual"
)

type LedgerEntry struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Type        string    `gorm:"size:10;not null" json:"type"` // credit | debit
	Amount      int64     `gorm:"not null" json:"amount"`       // in kobo, always positive
	Balance     int64     `gorm:"not null" json:"balance"`      // running balance after this entry
	Description string    `gorm:"size:500;not null" json:"description"`
	SourceType  string    `gorm:"size:50;not null" json:"source_type"`   // sale | expense | debt_payment | etc
	SourceID    uuid.UUID `gorm:"type:uuid;not null" json:"source_id"`   // ID of the source record
	RecordedBy  uuid.UUID `gorm:"type:uuid;not null" json:"recorded_by"` // user_id
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
