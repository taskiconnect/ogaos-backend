// internal/domain/models/business_identity.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	IdentityStatusUnverified = "unverified"
	IdentityStatusPending    = "pending"
	IdentityStatusVerified   = "verified"
	IdentityStatusRejected   = "rejected"
)

type BusinessIdentity struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"business_id"`
	CACNumber       *string    `gorm:"size:50" json:"cac_number"`
	TIN             *string    `gorm:"size:50" json:"tin"`
	BVN             *string    `gorm:"size:11" json:"-"` // never expose BVN in API responses
	CACDocumentURL  *string    `gorm:"size:500" json:"cac_document_url"`
	UtilityBillURL  *string    `gorm:"size:500" json:"utility_bill_url"`
	Status          string     `gorm:"size:20;not null;default:'unverified'" json:"status"`
	RejectionReason *string    `gorm:"type:text" json:"rejection_reason"`
	VerifiedAt      *time.Time `json:"verified_at"`
	VerifiedBy      *uuid.UUID `gorm:"type:uuid" json:"verified_by"` // platform_admin user_id
	SubmittedAt     *time.Time `json:"submitted_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
