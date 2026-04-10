// internal/service/public/service.go
package public

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

// ─── Cache ────────────────────────────────────────────────────────────────────

type cacheEntry struct {
	page      *models.PublicBusinessPage
	expiresAt time.Time
}

type cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func newCache() *cache {
	c := &cache{entries: make(map[string]cacheEntry)}
	go c.evict()
	return c
}

func (c *cache) get(key string) (*models.PublicBusinessPage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.page, true
}

func (c *cache) set(key string, page *models.PublicBusinessPage, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{page: page, expiresAt: time.Now().Add(ttl)}
}

func (c *cache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *cache) evict() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// ─── Service ─────────────────────────────────────────────────────────────────

const (
	cacheTTL              = 2 * time.Minute
	defaultSearchRadiusKM = 10.0
	maxSearchRadiusKM     = 50.0
	defaultSearchLimit    = 50
)

type Service struct {
	db    *gorm.DB
	cache *cache
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:    db,
		cache: newCache(),
	}
}

// ─── Public API ───────────────────────────────────────────────────────────────

func (s *Service) GetFullPage(slug string) (*models.PublicBusinessPage, error) {
	if cached, ok := s.cache.get(slug); ok {
		clone := *cached
		now := time.Now()
		clone.CachedAt = &now
		return &clone, nil
	}

	page, err := s.buildPage(slug)
	if err != nil {
		return nil, err
	}

	s.cache.set(slug, page, cacheTTL)
	return page, nil
}

func (s *Service) Invalidate(slug string) {
	s.cache.invalidate(slug)
}

// SearchBusinesses searches public businesses around the center of a selected LGA.
// It supports both keyword search and business-name search.
// If no result is found within the given radius and radius < 50 km,
// the response suggests expanding to 50 km.
func (s *Service) SearchBusinesses(query, state, lga string, radiusKM float64) (*models.PublicBusinessSearchResponse, error) {
	query = strings.TrimSpace(query)
	state = strings.TrimSpace(state)
	lga = strings.TrimSpace(lga)

	if state == "" || lga == "" {
		return nil, errors.New("state and lga are required")
	}

	if radiusKM <= 0 {
		radiusKM = defaultSearchRadiusKM
	}
	if radiusKM > maxSearchRadiusKM {
		return nil, fmt.Errorf("invalid radius: maximum allowed radius is %.0f km", maxSearchRadiusKM)
	}

	center, err := s.fetchLGACenter(state, lga)
	if err != nil {
		return nil, err
	}

	results, err := s.runBusinessSearch(center, query, radiusKM)
	if err != nil {
		return nil, err
	}

	meta := models.PublicBusinessSearchMeta{
		Query:              query,
		State:              center.State,
		LocalGovernment:    center.LocalGovernment,
		RadiusKM:           radiusKM,
		UsedFallbackRadius: almostEqual(radiusKM, maxSearchRadiusKM),
		Total:              len(results),
	}

	if len(results) == 0 && radiusKM < maxSearchRadiusKM {
		suggested := maxSearchRadiusKM
		meta.SuggestedExpandedRadiusKM = &suggested
	}

	return &models.PublicBusinessSearchResponse{
		Meta:    meta,
		Results: results,
	}, nil
}

// ─── Core Builder ─────────────────────────────────────────────────────────────

func (s *Service) buildPage(slug string) (*models.PublicBusinessPage, error) {
	biz, err := s.fetchBusiness(slug)
	if err != nil {
		return nil, err
	}

	keywords, err := s.fetchKeywords(biz.ID)
	if err != nil {
		return nil, err
	}

	digitalProducts, err := s.fetchDigitalProducts(biz.ID)
	if err != nil {
		return nil, err
	}

	physicalProducts, services, err := s.fetchPhysicalItems(biz.ID)
	if err != nil {
		return nil, err
	}

	go s.db.Model(&models.Business{}).
		Where("id = ?", biz.ID).
		UpdateColumn("profile_views", gorm.Expr("profile_views + 1"))

	page := &models.PublicBusinessPage{
		Business:         mapBusiness(biz, keywords),
		DigitalProducts:  digitalProducts,
		PhysicalProducts: physicalProducts,
		Services:         services,
		Stats: models.PublicStats{
			TotalProducts:        len(physicalProducts),
			TotalServices:        len(services),
			TotalDigitalProducts: len(digitalProducts),
		},
	}

	return page, nil
}

// ─── Existing Page Queries ───────────────────────────────────────────────────

func (s *Service) fetchBusiness(slug string) (*models.Business, error) {
	var biz models.Business
	err := s.db.
		Select("id, name, slug, category, description, logo_url, website_url, "+
			"street, city_town, local_government, state, country, "+
			"is_verified, profile_views, gallery_image_urls, storefront_video_url, is_profile_public").
		Where("slug = ? AND is_profile_public = true", slug).
		First(&biz).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("business not found or not public")
	}
	return &biz, err
}

func (s *Service) fetchKeywords(businessID uuid.UUID) ([]string, error) {
	type kwRow struct {
		Name string
	}

	var rows []kwRow
	err := s.db.
		Table("business_keywords").
		Select("keywords.name").
		Joins("JOIN keywords ON keywords.id = business_keywords.keyword_id").
		Where("business_keywords.business_id = ?", businessID).
		Order("keywords.name ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Name) != "" {
			names = append(names, r.Name)
		}
	}
	return names, nil
}

func (s *Service) fetchDigitalProducts(businessID uuid.UUID) ([]models.DigitalProductPublic, error) {
	type row struct {
		ID               uuid.UUID
		Title            string
		Slug             string
		Description      string
		Type             string
		Price            int64
		Currency         string
		FulfillmentMode  string
		CoverImageURL    *string
		GalleryImageURLs string
		PromoVideoURL    *string
		FileSize         *int64
		FileMimeType     *string
		DeliveryNote     *string
		SalesCount       int
		CreatedAt        time.Time
	}

	var rows []row
	err := s.db.
		Table("digital_products").
		Select("id, title, slug, description, type, price, currency, fulfillment_mode, "+
			"cover_image_url, gallery_image_urls, promo_video_url, "+
			"file_size, file_mime_type, delivery_note, sales_count, created_at").
		Where("business_id = ? AND is_published = true", businessID).
		Order("created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]models.DigitalProductPublic, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.DigitalProductPublic{
			ID:              r.ID,
			Title:           r.Title,
			Slug:            r.Slug,
			Description:     r.Description,
			Type:            r.Type,
			Price:           r.Price,
			Currency:        r.Currency,
			FulfillmentMode: r.FulfillmentMode,
			CoverImageURL:   r.CoverImageURL,
			GalleryImages:   r.GalleryImageURLs,
			PromoVideoURL:   r.PromoVideoURL,
			FileSize:        r.FileSize,
			FileMimeType:    r.FileMimeType,
			DeliveryNote:    r.DeliveryNote,
			SalesCount:      r.SalesCount,
			CreatedAt:       r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) fetchPhysicalItems(businessID uuid.UUID) (products []models.ProductPublic, services []models.ProductPublic, err error) {
	type row struct {
		ID                uuid.UUID
		Name              string
		Description       *string
		Type              string
		Price             int64
		ImageURL          *string
		SKU               *string
		TrackInventory    bool
		StockQuantity     int
		LowStockThreshold int
		CreatedAt         time.Time
	}

	var rows []row
	err = s.db.
		Table("products").
		Select("id, name, description, type, price, image_url, sku, "+
			"track_inventory, stock_quantity, low_stock_threshold, created_at").
		Where("business_id = ? AND is_active = true", businessID).
		Order("name ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}

	for _, r := range rows {
		inStock := !r.TrackInventory || r.StockQuantity > 0
		pub := models.ProductPublic{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			Type:        r.Type,
			Price:       r.Price,
			ImageURL:    r.ImageURL,
			SKU:         r.SKU,
			InStock:     inStock,
			CreatedAt:   r.CreatedAt,
		}

		if r.Type == models.ProductTypeService {
			services = append(services, pub)
		} else {
			products = append(products, pub)
		}
	}

	if products == nil {
		products = []models.ProductPublic{}
	}
	if services == nil {
		services = []models.ProductPublic{}
	}

	return products, services, nil
}

// ─── Search Queries ───────────────────────────────────────────────────────────

type lgaCenterRow struct {
	Country         string
	State           string
	LocalGovernment string
	Latitude        float64
	Longitude       float64
}

func (s *Service) fetchLGACenter(state, lga string) (*lgaCenterRow, error) {
	var row lgaCenterRow

	err := s.db.
		Table("local_government_centers").
		Select("country, state, local_government, latitude, longitude").
		Where("LOWER(TRIM(state)) = LOWER(TRIM(?)) AND LOWER(TRIM(local_government)) = LOWER(TRIM(?))", state, lga).
		First(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("location center not found for the selected state and lga")
	}
	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (s *Service) runBusinessSearch(center *lgaCenterRow, query string, radiusKM float64) ([]models.PublicBusinessSearchItem, error) {
	type searchRow struct {
		ID              uuid.UUID
		Name            string
		Slug            string
		Category        string
		Description     *string
		LogoURL         *string
		CityTown        string
		LocalGovernment string
		State           string
		Country         string
		IsVerified      bool
		DistanceKM      float64
		KeywordsCSV     *string
	}

	likeQuery := "%" + strings.TrimSpace(query) + "%"

	sql := `
WITH target_center AS (
	SELECT
		latitude AS target_lat,
		longitude AS target_lng
	FROM local_government_centers
	WHERE LOWER(TRIM(state)) = LOWER(TRIM(?))
	  AND LOWER(TRIM(local_government)) = LOWER(TRIM(?))
	LIMIT 1
),
business_centers AS (
	SELECT
		b.id,
		b.name,
		b.slug,
		b.category,
		b.description,
		b.logo_url,
		b.city_town,
		b.local_government,
		b.state,
		b.country,
		b.is_verified,
		lgc.latitude AS business_lat,
		lgc.longitude AS business_lng
	FROM businesses b
	JOIN local_government_centers lgc
	  ON LOWER(TRIM(lgc.state)) = LOWER(TRIM(b.state))
	 AND LOWER(TRIM(lgc.local_government)) = LOWER(TRIM(b.local_government))
	WHERE b.is_profile_public = true
),
ranked AS (
	SELECT
		bc.*,
		(
			6371.0 * ACOS(
				LEAST(
					1.0,
					GREATEST(
						-1.0,
						COS(RADIANS(tc.target_lat)) * COS(RADIANS(bc.business_lat)) *
						COS(RADIANS(bc.business_lng) - RADIANS(tc.target_lng)) +
						SIN(RADIANS(tc.target_lat)) * SIN(RADIANS(bc.business_lat))
					)
				)
			)
		) AS distance_km
	FROM business_centers bc
	CROSS JOIN target_center tc
)
SELECT
	r.id,
	r.name,
	r.slug,
	r.category,
	r.description,
	r.logo_url,
	r.city_town,
	r.local_government,
	r.state,
	r.country,
	r.is_verified,
	ROUND(r.distance_km::numeric, 2)::float8 AS distance_km,
	NULLIF(STRING_AGG(DISTINCT k.name, '||'), '') AS keywords_csv
FROM ranked r
LEFT JOIN business_keywords bk ON bk.business_id = r.id
LEFT JOIN keywords k ON k.id = bk.keyword_id
WHERE r.distance_km <= ?
  AND (
	? = '' OR
	r.name ILIKE ? OR
	r.category ILIKE ? OR
	COALESCE(r.description, '') ILIKE ? OR
	EXISTS (
		SELECT 1
		FROM business_keywords bk2
		JOIN keywords k2 ON k2.id = bk2.keyword_id
		WHERE bk2.business_id = r.id
		  AND k2.name ILIKE ?
	)
  )
GROUP BY
	r.id, r.name, r.slug, r.category, r.description, r.logo_url,
	r.city_town, r.local_government, r.state, r.country,
	r.is_verified, r.distance_km
ORDER BY
	r.distance_km ASC,
	r.is_verified DESC,
	r.name ASC
LIMIT ?
`

	var rows []searchRow
	err := s.db.Raw(
		sql,
		center.State,
		center.LocalGovernment,
		radiusKM,
		query,
		likeQuery,
		likeQuery,
		likeQuery,
		likeQuery,
		defaultSearchLimit,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]models.PublicBusinessSearchItem, 0, len(rows))
	for _, r := range rows {
		results = append(results, models.PublicBusinessSearchItem{
			ID:              r.ID,
			Name:            r.Name,
			Slug:            r.Slug,
			Category:        r.Category,
			Description:     r.Description,
			LogoURL:         r.LogoURL,
			CityTown:        r.CityTown,
			LocalGovernment: r.LocalGovernment,
			State:           r.State,
			Country:         r.Country,
			IsVerified:      r.IsVerified,
			DistanceKM:      round2(r.DistanceKM),
			Keywords:        splitKeywords(r.KeywordsCSV),
		})
	}

	if results == nil {
		results = []models.PublicBusinessSearchItem{}
	}

	return results, nil
}

// ─── Mappers ─────────────────────────────────────────────────────────────────

func mapBusiness(b *models.Business, keywords []string) models.BusinessPublic {
	if keywords == nil {
		keywords = []string{}
	}

	return models.BusinessPublic{
		ID:                 b.ID,
		Name:               b.Name,
		Slug:               b.Slug,
		Category:           b.Category,
		Description:        b.Description,
		LogoURL:            b.LogoURL,
		WebsiteURL:         b.WebsiteURL,
		Street:             b.Street,
		CityTown:           b.CityTown,
		LocalGovernment:    b.LocalGovernment,
		State:              b.State,
		Country:            b.Country,
		IsVerified:         b.IsVerified,
		ProfileViews:       b.ProfileViews,
		GalleryImageURLs:   b.GalleryImageURLs,
		StorefrontVideoURL: b.StorefrontVideoURL,
		Keywords:           keywords,
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func splitKeywords(csv *string) []string {
	if csv == nil || strings.TrimSpace(*csv) == "" {
		return []string{}
	}

	parts := strings.Split(*csv, "||")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}

	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func almostEqual(a, b float64) bool {
	const epsilon = 0.000001
	return math.Abs(a-b) < epsilon
}
