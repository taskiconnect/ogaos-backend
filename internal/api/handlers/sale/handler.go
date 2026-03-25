// internal/api/handlers/sale/handler.go
package sale

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcSale "ogaos-backend/internal/service/sale"
)

type Handler struct{ service *svcSale.Service }

func NewHandler(s *svcSale.Service) *Handler { return &Handler{service: s} }

// POST /sales
func (h *Handler) Create(c *gin.Context) {
	var req svcSale.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	sale, err := h.service.Create(shared.MustBusinessID(c), shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
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
		response.InternalError(c, err.Error())
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
		response.NotFound(c, err.Error())
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
		response.Err(c, err)
		return
	}
	response.OK(c, sale)
}

// POST /sales/:id/payment
// Body: { "amount": <kobo>, "payment_method": "cash|transfer|...", "note": "optional" }
// Supports installments — call multiple times until balance_due reaches 0.
func (h *Handler) RecordPayment(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcSale.RecordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	sale, err := h.service.RecordPayment(shared.MustBusinessID(c), id, shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, sale)
}

// PATCH /sales/:id/cancel
// Body: { "reason": "optional explanation" }
//
// Marks the sale as cancelled without deleting it — the record stays visible
// to the business owner so they can audit who cancelled it and why.
// All side-effects (stock, customer stats, debt, ledger) are reversed inside
// a single transaction so nothing is left in an inconsistent state.
func (h *Handler) Cancel(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcSale.CancelRequest
	// reason is optional — ignore bind errors
	_ = c.ShouldBindJSON(&req)

	sale, err := h.service.Cancel(shared.MustBusinessID(c), id, shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, sale)
}

// ─── private helpers ──────────────────────────────────────────────────────────

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
