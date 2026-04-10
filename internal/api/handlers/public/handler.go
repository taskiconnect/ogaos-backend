// internal/api/handlers/public/handler.go
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
		if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "not public") {
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
//	GET /public/business/search?q=tailor&state=Lagos&lga=Ikeja&radius_km=10
//
// Search origin is the center point of the selected LGA.
// If no result is found and radius_km < 50, the response suggests expanding to 50 km.
func (h *Handler) SearchBusinesses(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	state := strings.TrimSpace(c.Query("state"))
	lga := strings.TrimSpace(c.Query("lga"))

	if state == "" {
		response.BadRequest(c, "state is required")
		return
	}
	if lga == "" {
		response.BadRequest(c, "lga is required")
		return
	}

	radiusKM := 10.0
	if raw := strings.TrimSpace(c.Query("radius_km")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "radius_km must be a valid positive number")
			return
		}
		radiusKM = parsed
	}

	result, err := h.service.SearchBusinesses(query, state, lga, radiusKM)
	if err != nil {
		msg := strings.ToLower(err.Error())

		switch {
		case strings.Contains(msg, "location center not found"):
			response.NotFound(c, err.Error())
			return
		case strings.Contains(msg, "invalid radius"):
			response.BadRequest(c, err.Error())
			return
		default:
			response.InternalError(c, err.Error())
			return
		}
	}

	response.OK(c, result)
}
