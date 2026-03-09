// internal/domain/models/business_user.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type BusinessUser struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID uuid.UUID `gorm:"not null;index" json:"business_id"`
	UserID     uuid.UUID `gorm:"not null;index" json:"user_id"`
	Role       string    `gorm:"size:20;not null;default:'owner'" json:"role"`
	IsActive   bool      `gorm:"default:true" json:"is_active"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
