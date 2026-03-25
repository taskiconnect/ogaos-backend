package coupon

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ogaos-backend/internal/api/response"
	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/service/coupon"
)

type Handler struct {
	service *coupon.Service
}

func NewHandler(service *coupon.Service) *Handler {
	return &Handler{service: service}
}

// Create – ONLY PLATFORM ADMINS (already protected in routes)
func (h *Handler) Create(c *gin.Context) {
	var req struct {
		Code            string   `json:"code" binding:"required"`
		Description     string   `json:"description"`
		DiscountValue   int      `json:"discount_value" binding:"required,min=1,max=100"`
		ApplicablePlans []string `json:"applicable_plans" binding:"required"`
		StartsAt        string   `json:"starts_at" binding:"required"`
		ExpiresAt       string   `json:"expires_at" binding:"required"`
		MaxRedemptions  int      `json:"max_redemptions" binding:"min=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	startsAt, _ := time.Parse(time.RFC3339, req.StartsAt)
	expiresAt, _ := time.Parse(time.RFC3339, req.ExpiresAt)

	cpn := &models.Coupon{
		Code:            req.Code,
		Description:     req.Description,
		DiscountValue:   req.DiscountValue,
		ApplicablePlans: req.ApplicablePlans,
		StartsAt:        startsAt,
		ExpiresAt:       expiresAt,
		MaxRedemptions:  req.MaxRedemptions,
		CreatedBy:       c.MustGet("user_id").(uuid.UUID),
	}

	if err := h.service.Create(cpn); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Coupon created successfully", "data": cpn})
}

// List – shows all coupons + live redemption stats
func (h *Handler) List(c *gin.Context) {
	data, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// Get single coupon
func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}
	cpn, err := h.service.Get(id)
	if err != nil {
		response.NotFound(c, "coupon not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cpn})
}

// Update coupon
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.service.Update(id, updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon updated"})
}

// Delete (soft delete)
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}
	if err := h.service.Delete(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon deleted"})
}
