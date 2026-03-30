package coupon

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"ogaos-backend/internal/api/response"
	"ogaos-backend/internal/domain/models"
	apperr "ogaos-backend/internal/pkg/errors"
	"ogaos-backend/internal/service/coupon"
)

type Handler struct {
	service *coupon.Service
	log     *slog.Logger
}

func NewHandler(service *coupon.Service, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

func (h *Handler) Create(c *gin.Context) {
	var req struct {
		Code            string   `json:"code" binding:"required"`
		Description     string   `json:"description"`
		DiscountType    string   `json:"discount_type"`
		DiscountValue   int      `json:"discount_value" binding:"required,min=1,max=100"`
		ApplicablePlans []string `json:"applicable_plans" binding:"required"`
		StartsAt        string   `json:"starts_at" binding:"required"`
		ExpiresAt       string   `json:"expires_at" binding:"required"`
		MaxRedemptions  int      `json:"max_redemptions" binding:"min=0"`
		IsActive        *bool    `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		response.BadRequest(c, "starts_at must be a valid RFC3339 datetime")
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		response.BadRequest(c, "expires_at must be a valid RFC3339 datetime")
		return
	}

	adminIDValue, ok := c.Get("admin_id")
	if !ok {
		apperr.Respond(c, h.log, apperr.New(apperr.CodeUnauthorized, "authentication required"))
		return
	}

	adminID, ok := adminIDValue.(uuid.UUID)
	if !ok {
		apperr.Respond(c, h.log, apperr.New(apperr.CodeUnauthorized, "authentication required"))
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	cpn := &models.Coupon{
		Code:            req.Code,
		Description:     req.Description,
		DiscountType:    req.DiscountType,
		DiscountValue:   req.DiscountValue,
		ApplicablePlans: pq.StringArray(req.ApplicablePlans),
		StartsAt:        startsAt,
		ExpiresAt:       expiresAt,
		MaxRedemptions:  req.MaxRedemptions,
		IsActive:        isActive,
		CreatedBy:       adminID,
	}

	if err := h.service.Create(cpn); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.Created(c, cpn)
}

func (h *Handler) List(c *gin.Context) {
	data, err := h.service.List()
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, data)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}

	cpn, err := h.service.Get(id)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, cpn)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := h.service.Update(id, updates); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.Message(c, "coupon updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid coupon id")
		return
	}

	if err := h.service.Delete(id); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.Message(c, "coupon deleted successfully")
}
