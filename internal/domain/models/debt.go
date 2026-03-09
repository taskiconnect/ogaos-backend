// internal/domain/models/debt.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	DebtDirectionReceivable = "receivable" // customer owes business
	DebtDirectionPayable    = "payable"    // business owes supplier

	DebtStatusOutstanding = "outstanding"
	DebtStatusPartial     = "partial"
	DebtStatusSettled     = "settled"
	DebtStatusOverdue     = "overdue"
)

type Debt struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Direction  string    `gorm:"size:20;not null" json:"direction"` // receivable | payable
	// For receivables — who owes us
	CustomerID *uuid.UUID `gorm:"type:uuid;index" json:"customer_id"`
	// For payables — who we owe (supplier, not necessarily in our system)
	SupplierName  *string    `gorm:"size:255" json:"supplier_name"`
	SupplierPhone *string    `gorm:"size:20" json:"supplier_phone"`
	Description   string     `gorm:"type:text;not null" json:"description"`
	TotalAmount   int64      `gorm:"not null" json:"total_amount"` // in kobo
	AmountPaid    int64      `gorm:"default:0" json:"amount_paid"` // in kobo
	AmountDue     int64      `gorm:"not null" json:"amount_due"`   // in kobo (computed: total - paid)
	DueDate       *time.Time `gorm:"index" json:"due_date"`
	Status        string     `gorm:"size:20;not null;default:'outstanding'" json:"status"`
	Notes         *string    `gorm:"type:text" json:"notes"`
	RecordedBy    uuid.UUID  `gorm:"type:uuid;not null" json:"recorded_by"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business  `gorm:"foreignKey:BusinessID" json:"-"`
	Customer *Customer `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
}

// UpdateStatus recalculates debt status based on amounts
func (d *Debt) UpdateStatus() {
	switch {
	case d.AmountPaid >= d.TotalAmount:
		d.Status = DebtStatusSettled
		d.AmountDue = 0
	case d.AmountPaid > 0:
		d.Status = DebtStatusPartial
		d.AmountDue = d.TotalAmount - d.AmountPaid
	default:
		d.AmountDue = d.TotalAmount
		if d.DueDate != nil && time.Now().After(*d.DueDate) {
			d.Status = DebtStatusOverdue
		} else {
			d.Status = DebtStatusOutstanding
		}
	}
}
