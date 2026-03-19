// internal/domain/models/digital_product.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	DigitalProductTypeDownload = "download"
	DigitalProductTypeCourse   = "course"
	DigitalProductTypeVideo    = "video"
	DigitalProductTypeService  = "service"
	DigitalProductTypeOther    = "other"

	AllowedVideoHosts = "youtube.com,youtu.be,vimeo.com,drive.google.com"

	// Products are auto-unpublished 180 days after creation to manage storage.
	DigitalProductLifetimeDays = 180
)

type DigitalProduct struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index"                        json:"business_id"`
	Title       string    `gorm:"size:255;not null"                               json:"title"`
	Slug        string    `gorm:"size:300;not null"                               json:"slug"` // unique within business
	Description string    `gorm:"type:text;not null"                              json:"description"`
	Type        string    `gorm:"size:30;not null"                                json:"type"`
	Price       int64     `gorm:"not null"                                        json:"price"` // kobo
	Currency    string    `gorm:"size:5;default:'NGN'"                            json:"currency"`

	// Cover image — primary image shown in listings (e.g. thumbnail)
	CoverImageURL *string `gorm:"size:500" json:"cover_image_url"`

	// Gallery — up to 3 additional images stored as a JSON array string
	// e.g. '["https://...","https://..."]'
	GalleryImageURLs string `gorm:"type:text;default:'[]'" json:"gallery_image_urls"`

	// Promo video — link only (YouTube / Vimeo / Google Drive). No file upload allowed.
	PromoVideoURL *string `gorm:"size:500" json:"promo_video_url"`

	// The actual downloadable file — stored as private in ImageKit.
	// The URL is NEVER sent to the frontend (json:"-").
	FileURL      *string `gorm:"size:500" json:"-"`
	FileSize     *int64  `json:"file_size"`
	FileMimeType *string `gorm:"size:100" json:"file_mime_type"`

	IsPublished bool `gorm:"default:false;index" json:"is_published"`

	// ExpiresAt is set to created_at + 180 days on creation.
	// The scheduler checks this daily and unpublishes expired products.
	ExpiresAt *time.Time `gorm:"index" json:"expires_at"`

	// Denormalised stats — updated by the purchase flow
	SalesCount   int   `gorm:"default:0" json:"sales_count"`
	TotalRevenue int64 `gorm:"default:0" json:"total_revenue"` // kobo (owner's share after platform fee)

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// IsExpired returns true if the product has passed its storage expiry date.
func (p *DigitalProduct) IsExpired() bool {
	return p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt)
}
