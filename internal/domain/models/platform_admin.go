// internal/domain/models/platform_admin.go
package models

import (
	"time"

	"github.com/google/uuid"
)

// PlatformAdmin is an internal team member who manages the platform.
type PlatformAdmin struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Email              string     `gorm:"uniqueIndex;size:255;not null"                   json:"email"`
	FirstName          string     `gorm:"size:100;not null"                               json:"first_name"`
	LastName           string     `gorm:"size:100;not null"                               json:"last_name"`
	PasswordHash       string     `gorm:"not null"                                        json:"-"`
	Role               string     `gorm:"size:50;not null"                                json:"role"`
	IsActive           bool       `gorm:"default:true"                                    json:"is_active"`
	PasswordSetAt      *time.Time `                                                        json:"password_set_at"`
	LastLoginAt        *time.Time `                                                        json:"last_login_at"`
	PasswordResetToken *string    `gorm:"index"                                           json:"-"`
	ResetTokenExpires  *time.Time `                                                        json:"-"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"                                  json:"created_at"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"                                  json:"updated_at"`
}

// AdminRefreshToken stores hashed refresh tokens for platform admins.
// Completely separate from the users' refresh_tokens table.
type AdminRefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	TokenHash string    `gorm:"uniqueIndex;not null"                            json:"-"`
	AdminID   uuid.UUID `gorm:"type:uuid;index;not null"                        json:"admin_id"`
	ExpiresAt time.Time `gorm:"not null;index"                                  json:"expires_at"`
	Revoked   bool      `gorm:"default:false"                                   json:"revoked"`
	CreatedAt time.Time `gorm:"autoCreateTime"                                  json:"created_at"`
}

// AdminOTP is a one-time password record for admin 2FA.
type AdminOTP struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AdminID   uuid.UUID `gorm:"type:uuid;not null;index"                        json:"admin_id"`
	OTPHash   string    `gorm:"not null"                                        json:"-"`
	ExpiresAt time.Time `gorm:"not null"                                        json:"expires_at"`
	Used      bool      `gorm:"default:false;index"                             json:"used"`
	Attempts  int       `gorm:"default:0"                                       json:"attempts"`
	CreatedAt time.Time `gorm:"autoCreateTime"                                  json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"                                  json:"updated_at"`
}

// AdminOTPRateLimit tracks OTP send attempts per admin.
type AdminOTPRateLimit struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AdminID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"                  json:"admin_id"`
	Attempts      int       `gorm:"default:1"                                       json:"attempts"`
	LastAttemptAt time.Time `gorm:"default:now()"                                   json:"last_attempt_at"`
	CreatedAt     time.Time `gorm:"autoCreateTime"                                  json:"created_at"`
}
