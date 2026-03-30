// internal/domain/models/subscription.go
package models

import (
	"time"

	"github.com/google/uuid"
)

// Plan constants
const (
	PlanFree   = "free"
	PlanGrowth = "growth"
	PlanPro    = "pro"
	PlanCustom = "custom"
)

// Plan prices in kobo (placeholders — update when decided)
const (
	PriceGrowthMonthly = 0 // e.g. 500000 = ₦5,000
	PricePropMonthly   = 0 // e.g. 1000000 = ₦10,000
)

// Plan limits
var PlanLimits = map[string]PlanLimit{
	PlanFree: {
		MaxStaff:     2,
		MaxStores:    1,
		MaxProducts:  20,
		MaxCustomers: 50,
	},
	PlanGrowth: {
		MaxStaff:     5,
		MaxStores:    3,
		MaxProducts:  100,
		MaxCustomers: 500,
	},
	PlanPro: {
		MaxStaff:     10,
		MaxStores:    -1, // -1 = unlimited
		MaxProducts:  -1,
		MaxCustomers: -1,
	},
	PlanCustom: {
		MaxStaff:     -1,
		MaxStores:    -1,
		MaxProducts:  -1,
		MaxCustomers: -1,
	},
}

// PlanFeatures defines which features each plan unlocks
var PlanFeatures = map[string][]string{
	PlanFree: {
		"sales",
		"ledger",
		"customers_basic",
		"digital_store",
	},
	PlanGrowth: {
		"sales",
		"ledger",
		"customers_basic",
		"digital_store",
		"debt_tracking",
		"expense_tracking",
		"recruitment",
		"identity_kyc",
		"public_profile",
		"staff_management",
		"stores",
	},
	PlanPro: {
		"sales",
		"ledger",
		"customers_basic",
		"digital_store",
		"debt_tracking",
		"expense_tracking",
		"recruitment",
		"identity_kyc",
		"public_profile",
		"staff_management",
		"stores",
		"custom_domain",
		"priority_support",
	},
	PlanCustom: {
		"sales",
		"ledger",
		"customers_basic",
		"digital_store",
		"debt_tracking",
		"expense_tracking",
		"recruitment",
		"identity_kyc",
		"public_profile",
		"staff_management",
		"stores",
		"custom_domain",
		"priority_support",
	},
}

type PlanLimit struct {
	MaxStaff     int
	MaxStores    int
	MaxProducts  int
	MaxCustomers int
}

type Subscription struct {
	ID                       uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID               uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"business_id"`
	Plan                     string     `gorm:"size:20;not null;default:'free'" json:"plan"`
	Status                   string     `gorm:"size:20;not null;default:'active'" json:"status"` // active, expired, cancelled, grace_period
	PaystackSubscriptionCode *string    `gorm:"size:255" json:"-"`
	PaystackEmailToken       *string    `gorm:"size:255" json:"-"`
	CurrentPeriodStart       *time.Time `json:"current_period_start"`
	CurrentPeriodEnd         *time.Time `json:"current_period_end"`
	GracePeriodEndsAt        *time.Time `json:"grace_period_ends_at"`
	// Snapshot of limits at time of subscription
	MaxStaff     int       `gorm:"default:2" json:"max_staff"`
	MaxStores    int       `gorm:"default:1" json:"max_stores"`
	MaxProducts  int       `gorm:"default:20" json:"max_products"`
	MaxCustomers int       `gorm:"default:50" json:"max_customers"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// HasFeature checks if a subscription plan includes a given feature
func (s *Subscription) HasFeature(feature string) bool {
	features, ok := PlanFeatures[s.Plan]
	if !ok {
		return false
	}
	for _, f := range features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsActive returns true if subscription is usable (active or in grace period)
func (s *Subscription) IsActive() bool {
	return s.Status == "active" || s.Status == "grace_period"
}
