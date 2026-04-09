package business

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcBusiness "ogaos-backend/internal/service/business"
)

// GET /business/me/keywords
func (h *Handler) GetKeywords(c *gin.Context) {
	keywords, err := h.service.GetKeywords(shared.MustBusinessID(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"keywords": keywords})
}

// PUT /business/me/keywords
func (h *Handler) SetKeywords(c *gin.Context) {
	var req svcBusiness.SetKeywordsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	keywords, err := h.service.SetKeywords(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}

	response.OK(c, gin.H{"keywords": keywords})
}

// GET /public/business/:slug/keywords
func (h *Handler) GetPublicKeywords(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		response.BadRequest(c, "slug is required")
		return
	}

	keywords, err := h.service.GetKeywordsBySlug(slug)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "business not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"keywords": keywords})
}

// GET /public/keywords/suggestions?q=Fa
func (h *Handler) SuggestPublicKeywords(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		response.OK(c, gin.H{"keywords": []string{}})
		return
	}

	keywords, err := h.service.SuggestKeywords(query)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"keywords": keywords})
}
