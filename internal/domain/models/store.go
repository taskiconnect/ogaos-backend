// internal/domain/models/store.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Store struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	Description *string   `gorm:"type:text" json:"description"`
	Street      *string   `gorm:"size:255" json:"street"`
	CityTown    *string   `gorm:"size:100" json:"city_town"`
	State       *string   `gorm:"size:100" json:"state"`
	Phone       *string   `gorm:"size:20" json:"phone"`
	IsDefault   bool      `gorm:"default:false" json:"is_default"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
