// internal/domain/models/job_opening.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	JobTypeFullTime = "full_time"
	JobTypePartTime = "part_time"
	JobTypeContract = "contract"
	JobTypeIntern   = "internship"

	JobStatusOpen   = "open"
	JobStatusClosed = "closed"
	JobStatusDraft  = "draft"

	AssessmentCategoryGeneral         = "general"
	AssessmentCategoryCustomerService = "customer_service"
	AssessmentCategorySales           = "sales"
	AssessmentCategoryAdmin           = "admin"
	AssessmentCategoryTech            = "tech"
)

type JobOpening struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	PostedBy            uuid.UUID  `gorm:"type:uuid;not null" json:"posted_by"` // user_id
	Title               string     `gorm:"size:255;not null" json:"title"`
	Slug                string     `gorm:"size:300;not null" json:"slug"` // {business-slug}-{job-title-slug}
	Description         string     `gorm:"type:text;not null" json:"description"`
	Requirements        *string    `gorm:"type:text" json:"requirements"`
	Responsibilities    *string    `gorm:"type:text" json:"responsibilities"`
	Type                string     `gorm:"size:20;not null" json:"type"` // full_time | part_time | contract | internship
	Location            *string    `gorm:"size:255" json:"location"`
	IsRemote            bool       `gorm:"default:false" json:"is_remote"`
	SalaryRangeMin      *int64     `json:"salary_range_min"` // in kobo
	SalaryRangeMax      *int64     `json:"salary_range_max"` // in kobo
	ApplicationDeadline *time.Time `gorm:"index" json:"application_deadline"`
	Status              string     `gorm:"size:20;not null;default:'open'" json:"status"`
	// Assessment settings
	AssessmentEnabled  bool    `gorm:"default:false" json:"assessment_enabled"`
	AssessmentCategory *string `gorm:"size:50" json:"assessment_category"` // general | customer_service | sales | admin | tech
	PassThreshold      int     `gorm:"default:60" json:"pass_threshold"`   // percentage 0-100
	TimeLimitMinutes   int     `gorm:"default:30" json:"time_limit_minutes"`
	// Stats
	ApplicationCount int       `gorm:"default:0" json:"application_count"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business     Business                 `gorm:"foreignKey:BusinessID" json:"-"`
	Applications []RecruitmentApplication `gorm:"foreignKey:JobOpeningID" json:"applications,omitempty"`
}
