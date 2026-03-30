package sale

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	apperr "ogaos-backend/internal/pkg/errors"
	svcSale "ogaos-backend/internal/service/sale"
)

type Handler struct {
	service *svcSale.Service
	log     *slog.Logger
}

func NewHandler(s *svcSale.Service, log *slog.Logger) *Handler {
	return &Handler{
		service: s,
		log:     log,
	}
}

// POST /sales
func (h *Handler) Create(c *gin.Context) {
	var req svcSale.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	idempotencyKey := c.GetHeader("X-Idempotency-Key")

	sale, err := h.service.Create(
		shared.MustBusinessID(c),
		shared.MustUserID(c),
		req,
		idempotencyKey,
	)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.Created(c, sale)
}

// GET /sales
func (h *Handler) List(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	page := queryInt(c, "page", 1)

	sales, total, err := h.service.List(shared.MustBusinessID(c), svcSale.ListFilter{
		StoreID:    shared.QueryUUID(c, "store_id"),
		CustomerID: shared.QueryUUID(c, "customer_id"),
		Status:     c.Query("status"),
		DateFrom:   shared.QueryTime(c, "date_from"),
		DateTo:     shared.QueryTime(c, "date_to"),
		Page:       page,
		Limit:      limit,
	})
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    sales,
		"meta": gin.H{
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// GET /sales/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}

	sale, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, sale)
}

// POST /sales/:id/receipt
func (h *Handler) GenerateReceipt(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}

	sale, err := h.service.GenerateReceipt(shared.MustBusinessID(c), id)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, sale)
}

// POST /sales/:id/payment
func (h *Handler) RecordPayment(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}

	var req svcSale.RecordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	sale, err := h.service.RecordPayment(shared.MustBusinessID(c), id, shared.MustUserID(c), req)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, sale)
}

// PATCH /sales/:id/cancel
func (h *Handler) Cancel(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}

	var req svcSale.CancelRequest
	_ = c.ShouldBindJSON(&req)

	sale, err := h.service.Cancel(shared.MustBusinessID(c), id, shared.MustUserID(c), req)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	response.OK(c, sale)
}

func queryInt(c *gin.Context, key string, fallback int) int {
	s := c.Query(key)
	if s == "" {
		return fallback
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return fallback
	}

	return n
}
