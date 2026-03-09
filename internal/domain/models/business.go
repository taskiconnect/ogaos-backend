// internal/domain/models/business.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Business struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Name             string    `gorm:"size:255;not null" json:"name"`
	Slug             string    `gorm:"uniqueIndex;size:255;not null" json:"slug"`
	Category         string    `gorm:"size:100;not null" json:"category"`
	Description      *string   `gorm:"type:text" json:"description"`
	LogoURL          *string   `gorm:"size:500" json:"logo_url"`
	WebsiteURL       *string   `gorm:"size:500" json:"website_url"`
	Street           string    `gorm:"size:255" json:"street"`
	CityTown         string    `gorm:"size:100" json:"city_town"`
	LocalGovernment  string    `gorm:"size:100" json:"local_government"`
	State            string    `gorm:"size:100" json:"state"`
	Country          string    `gorm:"size:100" json:"country"`
	ReferralCodeUsed string    `gorm:"size:50" json:"referral_code_used"`
	Status           string    `gorm:"size:20;default:'active'" json:"status"`
	IsProfilePublic  bool      `gorm:"default:false" json:"is_profile_public"`
	ProfileViews     int64     `gorm:"default:0" json:"profile_views"`
	IsVerified       bool      `gorm:"default:false" json:"is_verified"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
