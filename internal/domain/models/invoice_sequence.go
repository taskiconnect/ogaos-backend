package models

import "github.com/google/uuid"

type BusinessInvoiceSequence struct {
	BusinessID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Period            string    `gorm:"size:6;primaryKey"` // YYYYMM
	LastInvoiceNumber int64     `gorm:"not null;default:0"`
}
