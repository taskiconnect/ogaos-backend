// internal/domain/models/sale_sequence.go
package models

import "github.com/google/uuid"

type BusinessSaleSequence struct {
	BusinessID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	LastSaleNumber    int64     `gorm:"not null;default:0"`
	LastReceiptNumber int64     `gorm:"not null;default:0"`
}
