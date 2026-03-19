// internal/service/business/service.go
package business

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type UpdateBusinessRequest struct {
	Name            *string `json:"name"`
	Description     *string `json:"description"`
	Category        *string `json:"category"`
	WebsiteURL      *string `json:"website_url"`
	Street          *string `json:"street"`
	CityTown        *string `json:"city_town"`
	LocalGovernment *string `json:"local_government"`
	State           *string `json:"state"`
	Country         *string `json:"country"`
}

// ─── Core methods ─────────────────────────────────────────────────────────────

// Get returns the business profile for the given businessID.
func (s *Service) Get(businessID uuid.UUID) (*models.Business, error) {
	var b models.Business
	if err := s.db.First(&b, businessID).Error; err != nil {
		return nil, errors.New("business not found")
	}
	return &b, nil
}

// Update updates editable business profile fields.
func (s *Service) Update(businessID uuid.UUID, req UpdateBusinessRequest) (*models.Business, error) {
	var b models.Business
	if err := s.db.First(&b, businessID).Error; err != nil {
		return nil, errors.New("business not found")
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
		updates["slug"] = generateSlug(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.WebsiteURL != nil {
		updates["website_url"] = *req.WebsiteURL
	}
	if req.Street != nil {
		updates["street"] = *req.Street
	}
	if req.CityTown != nil {
		updates["city_town"] = *req.CityTown
	}
	if req.LocalGovernment != nil {
		updates["local_government"] = *req.LocalGovernment
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}

	if err := s.db.Model(&b).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateLogo sets the logo_url after the upload service stores the file in ImageKit.
func (s *Service) UpdateLogo(businessID uuid.UUID, logoURL string) error {
	return s.db.Model(&models.Business{}).
		Where("id = ?", businessID).
		Update("logo_url", logoURL).Error
}

// SetProfilePublic toggles whether the business public profile is visible.
func (s *Service) SetProfilePublic(businessID uuid.UUID, isPublic bool) error {
	return s.db.Model(&models.Business{}).
		Where("id = ?", businessID).
		Update("is_profile_public", isPublic).Error
}

// GetPublicProfile returns a business by slug for the public storefront.
// Increments profile_views counter.
func (s *Service) GetPublicProfile(slug string) (*models.Business, error) {
	var b models.Business
	if err := s.db.Where("slug = ? AND is_profile_public = true", slug).First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}
	s.db.Model(&b).UpdateColumn("profile_views", gorm.Expr("profile_views + 1"))
	return &b, nil
}

// ─── Storefront gallery ───────────────────────────────────────────────────────

// AddGalleryImage adds an image URL to the business storefront gallery (max 3).
// Returns the updated business.
func (s *Service) AddGalleryImage(businessID uuid.UUID, imageURL string) (*models.Business, error) {
	var b models.Business
	if err := s.db.Where("id = ?", businessID).First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}
	gallery := parseGallery(b.GalleryImageURLs)
	if len(gallery) >= 3 {
		return nil, errors.New("maximum 3 gallery images allowed per storefront")
	}
	gallery = append(gallery, imageURL)
	if err := s.db.Model(&b).Update("gallery_image_urls", marshalGallery(gallery)).Error; err != nil {
		return nil, err
	}
	b.GalleryImageURLs = marshalGallery(gallery)
	return &b, nil
}

// RemoveGalleryImage removes a gallery image at the given zero-based index.
// Returns the updated business.
func (s *Service) RemoveGalleryImage(businessID uuid.UUID, index int) (*models.Business, error) {
	var b models.Business
	if err := s.db.Where("id = ?", businessID).First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}
	gallery := parseGallery(b.GalleryImageURLs)
	if index < 0 || index >= len(gallery) {
		return nil, errors.New("invalid gallery index")
	}
	gallery = append(gallery[:index], gallery[index+1:]...)
	if err := s.db.Model(&b).Update("gallery_image_urls", marshalGallery(gallery)).Error; err != nil {
		return nil, err
	}
	b.GalleryImageURLs = marshalGallery(gallery)
	return &b, nil
}

// SetStorefrontVideo sets or clears the business storefront promo video URL.
// Pass an empty string to clear the video.
func (s *Service) SetStorefrontVideo(businessID uuid.UUID, videoURL string) (*models.Business, error) {
	var b models.Business
	if err := s.db.Where("id = ?", businessID).First(&b).Error; err != nil {
		return nil, errors.New("business not found")
	}
	var val interface{}
	if videoURL == "" {
		val = nil
	} else {
		val = videoURL
	}
	if err := s.db.Model(&b).Update("storefront_video_url", val).Error; err != nil {
		return nil, err
	}
	if videoURL == "" {
		b.StorefrontVideoURL = nil
	} else {
		b.StorefrontVideoURL = &videoURL
	}
	return &b, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func parseGallery(raw string) []string {
	if raw == "" || raw == "null" {
		return []string{}
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return []string{}
	}
	return urls
}

func marshalGallery(urls []string) string {
	if urls == nil {
		urls = []string{}
	}
	b, _ := json.Marshal(urls)
	return string(b)
}

func generateSlug(name string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, name)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}
