// internal/domain/models/public.go
package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── Business ────────────────────────────────────────────────────────────────

// BusinessPublic is the safe, frontend-facing representation of a Business.
// Never expose internal fields like ReferralCodeUsed, Status, etc.
type BusinessPublic struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Slug               string    `json:"slug"`
	Category           string    `json:"category"`
	Description        *string   `json:"description"`
	LogoURL            *string   `json:"logo_url"`
	WebsiteURL         *string   `json:"website_url"`
	Street             string    `json:"street"`
	CityTown           string    `json:"city_town"`
	LocalGovernment    string    `json:"local_government"`
	State              string    `json:"state"`
	Country            string    `json:"country"`
	IsVerified         bool      `json:"is_verified"`
	ProfileViews       int64     `json:"profile_views"`
	GalleryImageURLs   string    `json:"gallery_image_urls"`
	StorefrontVideoURL *string   `json:"storefront_video_url"`
	Keywords           []string  `json:"keywords"`
}

// ─── Digital Products ────────────────────────────────────────────────────────

// DigitalProductPublic is the public-safe representation of a DigitalProduct.
// FileURL is intentionally excluded — buyers access files via signed tokens only.
type DigitalProductPublic struct {
	ID              uuid.UUID `json:"id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Description     string    `json:"description"`
	Type            string    `json:"type"`
	Price           int64     `json:"price"`
	Currency        string    `json:"currency"`
	FulfillmentMode string    `json:"fulfillment_mode"`
	CoverImageURL   *string   `json:"cover_image_url"`
	GalleryImages   string    `json:"gallery_image_urls"`
	PromoVideoURL   *string   `json:"promo_video_url"`
	FileSize        *int64    `json:"file_size"`      // bytes — useful for UI display
	FileMimeType    *string   `json:"file_mime_type"` // e.g. "application/pdf"
	DeliveryNote    *string   `json:"delivery_note"`
	SalesCount      int       `json:"sales_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// ─── Physical Products & Services ────────────────────────────────────────────

// ProductPublic is the public-safe representation of a physical Product or Service.
type ProductPublic struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Type        string    `json:"type"` // "product" | "service"
	Price       int64     `json:"price"`
	ImageURL    *string   `json:"image_url"`
	SKU         *string   `json:"sku"`
	InStock     bool      `json:"in_stock"` // derived: not out of stock OR inventory not tracked
	CreatedAt   time.Time `json:"created_at"`
}

// ─── Stats ───────────────────────────────────────────────────────────────────

type PublicStats struct {
	TotalProducts        int `json:"total_products"`
	TotalServices        int `json:"total_services"`
	TotalDigitalProducts int `json:"total_digital_products"`
}

// ─── Pagination ──────────────────────────────────────────────────────────────

type PublicPageCursors struct {
	DigitalProducts  *string `json:"digital_products,omitempty"`
	PhysicalProducts *string `json:"physical_products,omitempty"`
	Services         *string `json:"services,omitempty"`
}

// ─── Aggregated Page ─────────────────────────────────────────────────────────

type PublicBusinessPage struct {
	Business         BusinessPublic         `json:"business"`
	DigitalProducts  []DigitalProductPublic `json:"digital_products"`
	PhysicalProducts []ProductPublic        `json:"physical_products"`
	Services         []ProductPublic        `json:"services"`
	Stats            PublicStats            `json:"stats"`
	NextCursors      *PublicPageCursors     `json:"next_cursors,omitempty"`
	CachedAt         *time.Time             `json:"cached_at,omitempty"`
}

// ─── LGA Centers ─────────────────────────────────────────────────────────────

// LocalGovernmentCenter stores the approximate center point for an LGA.
// Search uses this instead of Google suggested addresses.
type LocalGovernmentCenter struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Country         string    `gorm:"type:text;not null;index:idx_lga_center_lookup,priority:1" json:"country"`
	State           string    `gorm:"type:text;not null;index:idx_lga_center_lookup,priority:2" json:"state"`
	LocalGovernment string    `gorm:"column:local_government;type:text;not null;index:idx_lga_center_lookup,priority:3" json:"local_government"`
	Latitude        float64   `gorm:"type:decimal(10,7);not null" json:"latitude"`
	Longitude       float64   `gorm:"type:decimal(10,7);not null" json:"longitude"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (LocalGovernmentCenter) TableName() string {
	return "local_government_centers"
}

// ─── Public Search ───────────────────────────────────────────────────────────

type PublicBusinessSearchItem struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Category        string    `json:"category"`
	Description     *string   `json:"description"`
	LogoURL         *string   `json:"logo_url"`
	CityTown        string    `json:"city_town"`
	LocalGovernment string    `json:"local_government"`
	State           string    `json:"state"`
	Country         string    `json:"country"`
	IsVerified      bool      `json:"is_verified"`
	Keywords        []string  `json:"keywords"`
	DistanceKM      float64   `json:"distance_km"`
}

type PublicBusinessSearchMeta struct {
	Query                     string   `json:"query"`
	State                     string   `json:"state"`
	LocalGovernment           string   `json:"local_government"`
	RadiusKM                  float64  `json:"radius_km"`
	UsedFallbackRadius        bool     `json:"used_fallback_radius"`
	SuggestedExpandedRadiusKM *float64 `json:"suggested_expanded_radius_km,omitempty"`
	Total                     int      `json:"total"`
}

type PublicBusinessSearchResponse struct {
	Meta    PublicBusinessSearchMeta   `json:"meta"`
	Results []PublicBusinessSearchItem `json:"results"`
}
