// internal/domain/models/user.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	FirstName             string     `gorm:"size:100;not null" json:"first_name"`
	LastName              string     `gorm:"size:100;not null" json:"last_name"`
	Email                 string     `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PhoneNumber           string     `gorm:"uniqueIndex;size:20;not null" json:"phone_number"`
	PasswordHash          string     `gorm:"not null" json:"-"`
	VerificationToken     *string    `gorm:"index" json:"-"`
	VerificationExpiresAt *time.Time `json:"-"`
	EmailVerifiedAt       *time.Time `gorm:"index" json:"email_verified_at"`
	IsActive              bool       `gorm:"default:true" json:"is_active"`
	CreatedAt             time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}
