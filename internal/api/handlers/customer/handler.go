// internal/api/handlers/customer/handler.go
package customer

import (
	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcCustomer "ogaos-backend/internal/service/customer"
)

type Handler struct{ service *svcCustomer.Service }

func NewHandler(s *svcCustomer.Service) *Handler { return &Handler{service: s} }

// POST /customers
func (h *Handler) Create(c *gin.Context) {
	var req svcCustomer.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	customer, err := h.service.Create(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, customer)
}

// GET /customers
func (h *Handler) List(c *gin.Context) {
	cur, limit := shared.CursorParams(c)
	customers, nextCursor, err := h.service.List(shared.MustBusinessID(c), svcCustomer.ListFilter{
		Search: c.Query("search"),
		Cursor: cur,
		Limit:  limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.CursorList(c, customers, nextCursor)
}

// GET /customers/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	customer, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, customer)
}

// PATCH /customers/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcCustomer.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	customer, err := h.service.Update(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, customer)
}

// DELETE /customers/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "customer deleted")
}
