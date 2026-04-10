package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	OrderPaymentStatusPending    = "pending"
	OrderPaymentStatusSuccessful = "successful"
	OrderPaymentStatusFailed     = "failed"

	PayoutStatusPending    = "pending"
	PayoutStatusProcessing = "processing"
	PayoutStatusCompleted  = "completed"
	PayoutStatusFailed     = "failed"

	DigitalFulfillmentStatusPending = "pending"
	DigitalFulfillmentStatusReady   = "ready"
	DigitalFulfillmentStatusRevoked = "revoked"

	PlatformFeePercent = 5
)

type DigitalOrder struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID       uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	DigitalProductID uuid.UUID `gorm:"type:uuid;not null;index" json:"digital_product_id"`

	BuyerName  string  `gorm:"size:200;not null" json:"buyer_name"`
	BuyerEmail string  `gorm:"size:255;not null;index" json:"buyer_email"`
	BuyerPhone *string `gorm:"size:20" json:"buyer_phone"`

	AmountPaid        int64      `gorm:"not null" json:"amount_paid"`
	PlatformFee       int64      `gorm:"not null" json:"platform_fee"`
	OwnerPayoutAmount int64      `gorm:"not null" json:"owner_payout_amount"`
	Currency          string     `gorm:"size:5;default:'NGN'" json:"currency"`
	PaymentChannel    string     `gorm:"size:30;not null" json:"payment_channel"`
	PaymentReference  *string    `gorm:"size:255;uniqueIndex" json:"payment_reference"`
	PaymentStatus     string     `gorm:"size:20;not null;default:'pending'" json:"payment_status"`
	PaidAt            *time.Time `json:"paid_at"`

	// Access / fulfillment
	FulfillmentMode   string     `gorm:"size:30;not null;default:'file_download'" json:"fulfillment_mode"`
	FulfillmentStatus string     `gorm:"size:20;not null;default:'pending'" json:"fulfillment_status"`
	AccessGranted     bool       `gorm:"default:false" json:"access_granted"`
	AccessToken       *string    `gorm:"size:255;uniqueIndex" json:"-"`
	AccessExpiresAt   *time.Time `json:"access_expires_at"`
	AccessURL         *string    `gorm:"size:500" json:"access_url"`
	DownloadCount     int        `gorm:"default:0" json:"download_count"`
	MaxDownloadCount  *int       `json:"max_download_count"`

	PayoutStatus      string     `gorm:"size:20;not null;default:'pending'" json:"payout_status"`
	PayoutReference   *string    `gorm:"size:255" json:"payout_reference"`
	PayoutAttempts    int        `gorm:"default:0" json:"payout_attempts"`
	PayoutCompletedAt *time.Time `json:"payout_completed_at"`
	PayoutFailReason  *string    `gorm:"type:text" json:"payout_fail_reason"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Business       Business       `gorm:"foreignKey:BusinessID" json:"-"`
	DigitalProduct DigitalProduct `gorm:"foreignKey:DigitalProductID" json:"product,omitempty"`
}

func (o *DigitalOrder) CalculateFees() {
	o.PlatformFee = (o.AmountPaid * PlatformFeePercent) / 100
	o.OwnerPayoutAmount = o.AmountPaid - o.PlatformFee
}
