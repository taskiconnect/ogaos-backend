// internal/domain/models/invoice_item.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type InvoiceItem struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	InvoiceID uuid.UUID  `gorm:"type:uuid;not null;index" json:"invoice_id"`
	ProductID *uuid.UUID `gorm:"type:uuid;index" json:"product_id"`

	// Free-text description — line items are not always catalogue products.
	// Examples: "Web design 3 hours", "Delivery fee", "CAC registration service"
	Description  string    `gorm:"size:500;not null" json:"description"`
	ProductSKU   *string   `gorm:"size:100" json:"product_sku"`
	UnitPrice    int64     `gorm:"not null" json:"unit_price"` // in kobo
	Quantity     int       `gorm:"not null;default:1" json:"quantity"`
	Discount     int64     `gorm:"default:0" json:"discount"`   // in kobo
	TotalPrice   int64     `gorm:"not null" json:"total_price"` // in kobo
	VATInclusive bool      `gorm:"default:false" json:"vat_inclusive"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Associations
	Invoice Invoice  `gorm:"foreignKey:InvoiceID" json:"-"`
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}
