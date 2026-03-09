// internal/domain/models/custom_domain.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	DomainStatusPending  = "pending"
	DomainStatusVerified = "verified"
	DomainStatusFailed   = "failed"
)

type CustomDomain struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"business_id"`
	Domain     string    `gorm:"size:255;not null;uniqueIndex" json:"domain"` // e.g. store.tundeventures.com
	Status     string    `gorm:"size:20;not null;default:'pending'" json:"status"`
	// DNS verification
	VerificationToken string     `gorm:"size:100;not null" json:"verification_token"` // TXT record value
	VerifiedAt        *time.Time `json:"verified_at"`
	// SSL
	SSLProvisioned   bool       `gorm:"default:false" json:"ssl_provisioned"`
	SSLProvisionedAt *time.Time `json:"ssl_provisioned_at"`
	LastCheckedAt    *time.Time `json:"last_checked_at"`
	FailReason       *string    `gorm:"type:text" json:"fail_reason"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
