// internal/api/handlers/invoice/handler.go
package invoice

import (
	"errors"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcInvoice "ogaos-backend/internal/service/invoice"
)

type Handler struct{ service *svcInvoice.Service }

func NewHandler(s *svcInvoice.Service) *Handler { return &Handler{service: s} }

// POST /invoices
func (h *Handler) Create(c *gin.Context) {
	var req svcInvoice.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	inv, err := h.service.Create(shared.MustBusinessID(c), shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, inv)
}

// GET /invoices
func (h *Handler) List(c *gin.Context) {
	cur, limit := shared.CursorParams(c)
	invoices, nextCursor, err := h.service.List(shared.MustBusinessID(c), svcInvoice.ListFilter{
		Status:     c.Query("status"),
		CustomerID: shared.QueryUUID(c, "customer_id"),
		DateFrom:   shared.QueryTime(c, "date_from"),
		DateTo:     shared.QueryTime(c, "date_to"),
		Cursor:     cur,
		Limit:      limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.CursorList(c, invoices, nextCursor)
}

// GET /invoices/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	inv, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		// FIX: distinguish not-found from internal errors so DB failures
		// don't masquerade as 404s.
		if errors.Is(err, svcInvoice.ErrNotFound) {
			response.NotFound(c, err.Error())
		} else {
			response.InternalError(c, err.Error())
		}
		return
	}
	response.OK(c, inv)
}

// POST /invoices/:id/send
func (h *Handler) Send(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	inv, err := h.service.Send(shared.MustBusinessID(c), id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, inv)
}

// POST /invoices/:id/payment
func (h *Handler) RecordPayment(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Amount int64 `json:"amount" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	inv, err := h.service.RecordPayment(shared.MustBusinessID(c), id, req.Amount)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, inv)
}

// DELETE /invoices/:id
func (h *Handler) Cancel(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Cancel(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "invoice cancelled")
}
