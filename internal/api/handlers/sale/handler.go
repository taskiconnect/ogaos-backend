// internal/api/handlers/sale/handler.go
package sale

import (
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
	page, limit := shared.Paginate(c)
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
	response.List(c, sales, total, page, limit)
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
