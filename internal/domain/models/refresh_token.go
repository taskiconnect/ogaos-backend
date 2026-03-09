// internal/domain/models/refresh_token.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	TokenHash string    `gorm:"uniqueIndex;not null" json:"-"`
	UserID    uuid.UUID `gorm:"index" json:"user_id"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	Revoked   bool      `gorm:"default:false" json:"revoked"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
