// internal/domain/models/staff_profile.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type StaffProfile struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Position              *string    `gorm:"size:100" json:"position"`
	Department            *string    `gorm:"size:100" json:"department"`
	Salary                *int64     `json:"salary"` // in kobo, optional
	StartDate             *time.Time `json:"start_date"`
	EmergencyContactName  *string    `gorm:"size:200" json:"emergency_contact_name"`
	EmergencyContactPhone *string    `gorm:"size:20" json:"emergency_contact_phone"`
	Notes                 *string    `gorm:"type:text" json:"notes"`
	CreatedAt             time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
	User     User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
