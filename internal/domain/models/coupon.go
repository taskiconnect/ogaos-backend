package models

import (
	"net"
	"time"

	"github.com/google/uuid"
)

// Coupon represents a promotional coupon code (enhanced for multi-use)
type Coupon struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Code            string    `gorm:"size:50;uniqueIndex;not null" json:"code"`
	Description     string    `gorm:"type:text" json:"description"`
	DiscountType    string    `gorm:"size:10;not null;default:'percentage'" json:"discount_type"`
	DiscountValue   int       `gorm:"not null;check:discount_value BETWEEN 1 AND 100" json:"discount_value"`
	ApplicablePlans []string  `gorm:"type:text[];not null" json:"applicable_plans"`

	// Validity
	StartsAt  time.Time `gorm:"not null" json:"starts_at"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`

	// Multi-use (new)
	MaxRedemptions int `gorm:"default:1" json:"max_redemptions"` // 0 = unlimited

	// Usage tracking (kept for backward compatibility)
	IsUsed               bool       `gorm:"default:false;index:idx_coupons_is_used" json:"is_used"`
	UsedByBusinessID     *uuid.UUID `gorm:"type:uuid" json:"used_by_business_id,omitempty"`
	UsedAt               *time.Time `json:"used_at,omitempty"`
	UsedOnSubscriptionID *uuid.UUID `gorm:"type:uuid" json:"used_on_subscription_id,omitempty"`

	// Status + soft delete
	IsActive  bool       `gorm:"default:true" json:"is_active"`
	DeletedAt *time.Time `gorm:"index" json:"-"`

	// Audit
	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	UsedByBusiness     *Business      `gorm:"foreignKey:UsedByBusinessID" json:"-"`
	UsedOnSubscription *Subscription  `gorm:"foreignKey:UsedOnSubscriptionID" json:"-"`
	Creator            *PlatformAdmin `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (Coupon) TableName() string { return "coupons" }

// IsValid checks if coupon is currently valid (date, active, not soft-deleted, and under redemption limit)
func (c *Coupon) IsValid(redemptionCount int) bool {
	if !c.IsActive || c.DeletedAt != nil {
		return false
	}
	now := time.Now()
	if now.Before(c.StartsAt) || now.After(c.ExpiresAt) {
		return false
	}
	if c.MaxRedemptions > 0 && redemptionCount >= c.MaxRedemptions {
		return false
	}
	return true
}

// IsPlanEligible checks if coupon applies to given plan
func (c *Coupon) IsPlanEligible(plan string) bool {
	for _, p := range c.ApplicablePlans {
		if p == plan || p == "all" {
			return true
		}
	}
	return false
}

// CalculateDiscount returns the discounted amount (in kobo)
func (c *Coupon) CalculateDiscount(originalAmount int64) int64 {
	if c.DiscountType != "percentage" {
		return 0
	}
	discount := (originalAmount * int64(c.DiscountValue)) / 100
	if discount > originalAmount {
		return originalAmount
	}
	return discount
}

// CouponRedemption (unchanged - already perfect)
type CouponRedemption struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	CouponID       uuid.UUID `gorm:"type:uuid;not null" json:"coupon_id"`
	BusinessID     uuid.UUID `gorm:"type:uuid;not null" json:"business_id"`
	SubscriptionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"subscription_id"`

	SubscriptionPlan string `gorm:"size:20;not null" json:"subscription_plan"`
	OriginalAmount   int64  `gorm:"not null" json:"original_amount"`
	DiscountAmount   int64  `gorm:"not null" json:"discount_amount"`
	FinalAmount      int64  `gorm:"not null" json:"final_amount"`

	PaymentReference string `gorm:"size:255" json:"payment_reference,omitempty"`
	PaymentChannel   string `gorm:"size:30" json:"payment_channel,omitempty"`

	RedeemedAt time.Time `gorm:"autoCreateTime" json:"redeemed_at"`
	IPAddress  net.IP    `gorm:"type:inet" json:"ip_address,omitempty"`
	UserAgent  string    `gorm:"type:text" json:"user_agent,omitempty"`

	Coupon       *Coupon       `gorm:"foreignKey:CouponID" json:"-"`
	Business     *Business     `gorm:"foreignKey:BusinessID" json:"-"`
	Subscription *Subscription `gorm:"foreignKey:SubscriptionID" json:"-"`
}

func (CouponRedemption) TableName() string { return "coupon_redemptions" }
