// internal/domain/models/digital_product.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	DigitalProductTypeDownload = "download" // ebook, template, file
	DigitalProductTypeCourse   = "course"
	DigitalProductTypeVideo    = "video"
	DigitalProductTypeService  = "service" // consulting, coaching session
	DigitalProductTypeOther    = "other"

	AllowedVideoHosts = "youtube.com,youtu.be,vimeo.com,drive.google.com"
)

type DigitalProduct struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Title         string    `gorm:"size:255;not null" json:"title"`
	Slug          string    `gorm:"size:300;not null" json:"slug"` // unique within business
	Description   string    `gorm:"type:text;not null" json:"description"`
	Type          string    `gorm:"size:30;not null" json:"type"`
	Price         int64     `gorm:"not null" json:"price"` // in kobo
	Currency      string    `gorm:"size:5;default:'NGN'" json:"currency"`
	CoverImageURL *string   `gorm:"size:500" json:"cover_image_url"` // ImageKit public
	PromoVideoURL *string   `gorm:"size:500" json:"promo_video_url"` // YouTube/Vimeo URL
	FileURL       *string   `gorm:"size:500" json:"-"`               // ImageKit private — never expose directly
	FileSize      *int64    `json:"file_size"`                       // in bytes
	FileMimeType  *string   `gorm:"size:100" json:"file_mime_type"`
	IsPublished   bool      `gorm:"default:false;index" json:"is_published"`
	// Denormalized stats
	SalesCount   int       `gorm:"default:0" json:"sales_count"`
	TotalRevenue int64     `gorm:"default:0" json:"total_revenue"` // in kobo (owner's share)
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
