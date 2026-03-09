// internal/domain/models/customer.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	FirstName   string    `gorm:"size:100;not null" json:"first_name"`
	LastName    string    `gorm:"size:100;not null" json:"last_name"`
	Email       *string   `gorm:"size:255;index" json:"email"`
	PhoneNumber *string   `gorm:"size:20;index" json:"phone_number"`
	Address     *string   `gorm:"type:text" json:"address"`
	Notes       *string   `gorm:"type:text" json:"notes"`
	// Denormalized stats — updated on each sale/debt event
	TotalPurchases  int64     `gorm:"default:0" json:"total_purchases"` // in kobo
	TotalOrders     int       `gorm:"default:0" json:"total_orders"`
	OutstandingDebt int64     `gorm:"default:0" json:"outstanding_debt"` // in kobo
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// FullName returns the customer's full name
func (c *Customer) FullName() string {
	return c.FirstName + " " + c.LastName
}
