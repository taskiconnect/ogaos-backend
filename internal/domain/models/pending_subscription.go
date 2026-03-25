package models

import (
	"time"

	"github.com/google/uuid"
)

type PendingSubscription struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID     uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Reference      string    `gorm:"size:100;uniqueIndex;not null" json:"reference"`
	Plan           string    `gorm:"size:20;not null" json:"plan"`
	PeriodMonths   int       `gorm:"not null" json:"period_months"`
	OriginalAmount int64     `gorm:"not null" json:"original_amount"` // kobo
	FinalAmount    int64     `gorm:"not null" json:"final_amount"`    // kobo
	CouponCode     *string   `gorm:"size:50" json:"coupon_code,omitempty"`
	Status         string    `gorm:"size:20;default:'pending'" json:"status"` // pending, completed, expired, failed
	ExpiresAt      time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

func (PendingSubscription) TableName() string { return "pending_subscriptions" }
