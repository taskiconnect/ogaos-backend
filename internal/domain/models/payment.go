// internal/domain/models/payment.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	PaymentSourceSale = "sale"
	PaymentSourceDebt = "debt"

	PaymentStatusPending    = "pending"
	PaymentStatusSuccessful = "successful"
	PaymentStatusFailed     = "failed"
	PaymentStatusRefunded   = "refunded"
)

type Payment struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	SourceType string    `gorm:"size:20;not null" json:"source_type"` // sale | debt
	SourceID   uuid.UUID `gorm:"type:uuid;not null;index" json:"source_id"`
	Amount     int64     `gorm:"not null" json:"amount"`                // in kobo
	Channel    string    `gorm:"size:30;not null" json:"channel"`       // cash | transfer | pos | paystack | flutterwave
	Reference  *string   `gorm:"size:255;uniqueIndex" json:"reference"` // gateway ref
	Status     string    `gorm:"size:20;not null;default:'successful'" json:"status"`
	Note       *string   `gorm:"type:text" json:"note"`
	RecordedBy uuid.UUID `gorm:"type:uuid;not null" json:"recorded_by"`
	PaidAt     time.Time `gorm:"not null" json:"paid_at"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
