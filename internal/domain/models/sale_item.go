// internal/domain/models/sale_item.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type SaleItem struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	SaleID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"sale_id"`
	ProductID *uuid.UUID `gorm:"type:uuid;index" json:"product_id"` // nullable — item may be deleted later
	// Snapshot of product details at time of sale
	ProductName string    `gorm:"size:255;not null" json:"product_name"`
	ProductSKU  *string   `gorm:"size:100" json:"product_sku"`
	UnitPrice   int64     `gorm:"not null" json:"unit_price"` // in kobo
	Quantity    int       `gorm:"not null;default:1" json:"quantity"`
	Discount    int64     `gorm:"default:0" json:"discount"`   // in kobo
	TotalPrice  int64     `gorm:"not null" json:"total_price"` // in kobo
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Associations
	Sale    Sale     `gorm:"foreignKey:SaleID" json:"-"`
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}
