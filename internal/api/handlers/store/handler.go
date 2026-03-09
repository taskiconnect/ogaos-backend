// internal/api/handlers/store/handler.go
package store

import (
	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcStore "ogaos-backend/internal/service/store"
)

type Handler struct{ service *svcStore.Service }

func NewHandler(s *svcStore.Service) *Handler { return &Handler{service: s} }

// POST /stores
func (h *Handler) Create(c *gin.Context) {
	var req svcStore.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	store, err := h.service.Create(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, store)
}

// GET /stores
func (h *Handler) List(c *gin.Context) {
	stores, err := h.service.List(shared.MustBusinessID(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, stores)
}

// GET /stores/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	store, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, store)
}

// PATCH /stores/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcStore.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	store, err := h.service.Update(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, store)
}

// PATCH /stores/:id/default
func (h *Handler) SetDefault(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.SetDefault(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "default store updated")
}

// DELETE /stores/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "store deleted")
}
