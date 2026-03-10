// internal/api/handlers/debt/handler.go
package debt

import (
	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcDebt "ogaos-backend/internal/service/debt"
)

type Handler struct{ service *svcDebt.Service }

func NewHandler(s *svcDebt.Service) *Handler { return &Handler{service: s} }

// POST /debts
func (h *Handler) Create(c *gin.Context) {
	var req svcDebt.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	d, err := h.service.Create(shared.MustBusinessID(c), shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, d)
}

// GET /debts
func (h *Handler) List(c *gin.Context) {
	cur, limit := shared.CursorParams(c)
	debts, nextCursor, err := h.service.List(shared.MustBusinessID(c), svcDebt.ListFilter{
		Direction:  c.Query("direction"),
		Status:     c.Query("status"),
		CustomerID: shared.QueryUUID(c, "customer_id"),
		Overdue:    shared.QueryBool(c, "overdue"),
		Cursor:     cur,
		Limit:      limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.CursorList(c, debts, nextCursor)
}

// GET /debts/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	d, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, d)
}

// POST /debts/:id/payment
func (h *Handler) RecordPayment(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcDebt.RecordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	d, err := h.service.RecordPayment(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, d)
}
