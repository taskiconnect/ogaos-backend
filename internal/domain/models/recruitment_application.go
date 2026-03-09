// internal/domain/models/recruitment_application.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	ApplicationStatusNew         = "new"
	ApplicationStatusReviewing   = "reviewing"
	ApplicationStatusShortlisted = "shortlisted"
	ApplicationStatusRejected    = "rejected"
	ApplicationStatusHired       = "hired"

	AssessmentStatusNotRequired = "not_required"
	AssessmentStatusPending     = "pending"
	AssessmentStatusInProgress  = "in_progress"
	AssessmentStatusCompleted   = "completed"
	AssessmentStatusExpired     = "expired"
)

type RecruitmentApplication struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID   uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	JobOpeningID uuid.UUID `gorm:"type:uuid;not null;index" json:"job_opening_id"`
	// Applicant details
	FirstName   string  `gorm:"size:100;not null" json:"first_name"`
	LastName    string  `gorm:"size:100;not null" json:"last_name"`
	Email       string  `gorm:"size:255;not null;index" json:"email"`
	PhoneNumber string  `gorm:"size:20;not null" json:"phone_number"`
	CoverLetter *string `gorm:"type:text" json:"cover_letter"`
	CVUrl       *string `gorm:"size:500" json:"cv_url"` // ImageKit URL
	// Application status
	Status      string  `gorm:"size:30;not null;default:'new'" json:"status"`
	ReviewNotes *string `gorm:"type:text" json:"review_notes"`
	// Assessment
	AssessmentStatus      string     `gorm:"size:20;not null;default:'not_required'" json:"assessment_status"`
	AssessmentScore       *int       `json:"assessment_score"`  // 0-100, null until completed
	AssessmentPassed      *bool      `json:"assessment_passed"` // null until completed
	AssessmentCompletedAt *time.Time `json:"assessment_completed_at"`
	CreatedAt             time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business   Business   `gorm:"foreignKey:BusinessID" json:"-"`
	JobOpening JobOpening `gorm:"foreignKey:JobOpeningID" json:"job_opening,omitempty"`
}

// FullName returns applicant's full name
func (a *RecruitmentApplication) FullName() string {
	return a.FirstName + " " + a.LastName
}
