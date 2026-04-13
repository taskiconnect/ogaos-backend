package public

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/response"
	svcPublic "ogaos-backend/internal/service/public"
)

// Handler serves the aggregated public storefront endpoints.
type Handler struct {
	service *svcPublic.Service
}

func NewHandler(service *svcPublic.Service) *Handler {
	return &Handler{service: service}
}

// GetFullBusinessPage handles:
//
//	GET /public/business/:slug/full
func (h *Handler) GetFullBusinessPage(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		response.BadRequest(c, "slug is required")
		return
	}

	page, err := h.service.GetFullPage(slug)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not public") {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, page)
}

// SearchBusinesses handles:
//
//	GET /public/business/search?q=tech&lat=6.5244&lng=3.3792&radius_km=10
//
// If lat/lng are provided, businesses are searched by coordinate radius.
// If lat/lng are omitted (for example, user denied location), the endpoint
// returns all public businesses matching the keyword query.
func (h *Handler) SearchBusinesses(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))

	var (
		lat      *float64
		lng      *float64
		radiusKM = 10.0
	)

	if raw := strings.TrimSpace(c.Query("radius_km")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "radius_km must be a valid positive number")
			return
		}
		radiusKM = parsed
	}

	rawLat := strings.TrimSpace(c.Query("lat"))
	rawLng := strings.TrimSpace(c.Query("lng"))

	if rawLat != "" || rawLng != "" {
		if rawLat == "" || rawLng == "" {
			response.BadRequest(c, "both lat and lng are required when using coordinate search")
			return
		}

		parsedLat, err := strconv.ParseFloat(rawLat, 64)
		if err != nil {
			response.BadRequest(c, "lat must be a valid number")
			return
		}

		parsedLng, err := strconv.ParseFloat(rawLng, 64)
		if err != nil {
			response.BadRequest(c, "lng must be a valid number")
			return
		}

		lat = &parsedLat
		lng = &parsedLng
	}

	result, err := h.service.SearchBusinesses(query, lat, lng, radiusKM)
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "invalid radius"),
			strings.Contains(msg, "invalid latitude"),
			strings.Contains(msg, "invalid longitude"),
			strings.Contains(msg, "both latitude and longitude"):
			response.BadRequest(c, err.Error())
			return
		default:
			response.InternalError(c, err.Error())
			return
		}
	}

	response.OK(c, result)
}
