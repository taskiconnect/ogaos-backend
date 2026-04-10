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

	DigitalFulfillmentModeFileDownload   = "file_download"
	DigitalFulfillmentModeCourseAccess   = "course_access"
	DigitalFulfillmentModeExternalLink   = "external_link"
	DigitalFulfillmentModeManualDelivery = "manual_delivery"

	AllowedVideoHosts = "youtube.com,youtu.be,vimeo.com,drive.google.com"
)

type DigitalProduct struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Title       string    `gorm:"size:255;not null" json:"title"`
	Slug        string    `gorm:"size:300;not null" json:"slug"`
	Description string    `gorm:"type:text;not null" json:"description"`
	Type        string    `gorm:"size:30;not null" json:"type"`
	Price       int64     `gorm:"not null" json:"price"`
	Currency    string    `gorm:"size:5;default:'NGN'" json:"currency"`

	// Fulfillment / access
	FulfillmentMode     string  `gorm:"size:30;not null;default:'file_download'" json:"fulfillment_mode"`
	AccessRedirectURL   *string `gorm:"size:500" json:"access_redirect_url"`
	RequiresAccount     bool    `gorm:"default:false" json:"requires_account"`
	AccessDurationHours *int    `json:"access_duration_hours"`
	DeliveryNote        *string `gorm:"type:text" json:"delivery_note"`

	// Media
	CoverImageURL    *string `gorm:"size:500" json:"cover_image_url"`
	GalleryImageURLs string  `gorm:"type:text;default:'[]'" json:"gallery_image_urls"`
	PromoVideoURL    *string `gorm:"size:500" json:"promo_video_url"`

	// Private downloadable file
	FileURL      *string `gorm:"size:500" json:"-"`
	FileSize     *int64  `json:"file_size"`
	FileMimeType *string `gorm:"size:100" json:"file_mime_type"`

	IsPublished bool `gorm:"default:false;index" json:"is_published"`

	// Stats
	SalesCount   int   `gorm:"default:0" json:"sales_count"`
	TotalRevenue int64 `gorm:"default:0" json:"total_revenue"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}
