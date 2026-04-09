package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProductTypeProduct = "product"
	ProductTypeService = "service"
)

type Product struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID     uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_business_barcode" json:"business_id"`
	StoreID        *uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	Name           string     `gorm:"size:255;not null" json:"name"`
	Description    *string    `gorm:"type:text" json:"description"`
	Type           string     `gorm:"size:20;not null;default:'product'" json:"type"`
	SKU            *string    `gorm:"size:100;index" json:"sku"`
	Price          int64      `gorm:"not null" json:"price"`
	CostPrice      *int64     `json:"cost_price"`
	ImageURL       *string    `gorm:"size:500" json:"image_url"`
	Barcode        *string    `gorm:"size:100;uniqueIndex:idx_business_barcode" json:"barcode"`
	IdempotencyKey *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"-"`

	TrackInventory    bool      `gorm:"default:false" json:"track_inventory"`
	StockQuantity     int       `gorm:"default:0" json:"stock_quantity"`
	LowStockThreshold int       `gorm:"default:5" json:"low_stock_threshold"`
	IsActive          bool      `gorm:"default:true" json:"is_active"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
	Store    *Store   `gorm:"foreignKey:StoreID" json:"-"`
}

func (p *Product) IsLowStock() bool {
	return p.TrackInventory && p.StockQuantity <= p.LowStockThreshold
}

func (p *Product) IsOutOfStock() bool {
	return p.TrackInventory && p.StockQuantity <= 0
}
