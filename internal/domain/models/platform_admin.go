// internal/domain/models/platform_admin.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type PlatformAdmin struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Email        string     `gorm:"uniqueIndex;size:255;not null" json:"email"`
	FirstName    string     `gorm:"size:100;not null" json:"first_name"`
	LastName     string     `gorm:"size:100;not null" json:"last_name"`
	PasswordHash string     `gorm:"not null" json:"-"`
	Role         string     `gorm:"size:50;not null" json:"role"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}
