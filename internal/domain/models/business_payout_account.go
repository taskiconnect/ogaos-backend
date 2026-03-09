// internal/domain/models/business_payout_account.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type BusinessPayoutAccount struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	BankName      string    `gorm:"size:100;not null" json:"bank_name"`
	BankCode      string    `gorm:"size:10;not null" json:"bank_code"` // Paystack bank code
	AccountNumber string    `gorm:"size:20;not null" json:"account_number"`
	AccountName   string    `gorm:"size:255;not null" json:"account_name"` // verified via Paystack
	// Paystack recipient code — used to initiate transfers
	PaystackRecipientCode *string   `gorm:"size:100" json:"-"`
	IsVerified            bool      `gorm:"default:false" json:"is_verified"`
	IsDefault             bool      `gorm:"default:true" json:"is_default"`
	CreatedAt             time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
