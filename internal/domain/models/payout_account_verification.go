package models

import (
	"time"

	"github.com/google/uuid"
)

type PayoutAccountVerification struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	BankName      string    `gorm:"size:100;not null" json:"bank_name"`
	BankCode      string    `gorm:"size:10;not null" json:"bank_code"`
	AccountNumber string    `gorm:"size:20;not null" json:"account_number"`
	AccountName   string    `gorm:"size:255;not null" json:"account_name"`

	OTPHash     string    `gorm:"size:255;not null" json:"-"`
	ExpiresAt   time.Time `gorm:"not null;index" json:"expires_at"`
	ResendAfter time.Time `gorm:"not null" json:"resend_after"`
	Attempts    int       `gorm:"default:0" json:"attempts"`
	MaxAttempts int       `gorm:"default:5" json:"max_attempts"`
	IsVerified  bool      `gorm:"default:false" json:"is_verified"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
