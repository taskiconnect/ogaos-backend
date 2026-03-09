// internal/api/handlers/product/handler.go
package product

import (
	"io"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcProduct "ogaos-backend/internal/service/product"
	svcUpload "ogaos-backend/internal/service/upload"
)

type Handler struct {
	service *svcProduct.Service
	upload  *svcUpload.Service
}

func NewHandler(s *svcProduct.Service, u *svcUpload.Service) *Handler {
	return &Handler{service: s, upload: u}
}

// POST /products
func (h *Handler) Create(c *gin.Context) {
	var req svcProduct.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.service.Create(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, p)
}

// GET /products
func (h *Handler) List(c *gin.Context) {
	page, limit := shared.Paginate(c)
	products, total, err := h.service.List(shared.MustBusinessID(c), svcProduct.ListFilter{
		StoreID:  shared.QueryUUID(c, "store_id"),
		Type:     c.Query("type"),
		Search:   c.Query("search"),
		LowStock: shared.QueryBool(c, "low_stock"),
		Page:     page,
		Limit:    limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.List(c, products, total, page, limit)
}

// GET /products/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	p, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, p)
}

// PATCH /products/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcProduct.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.service.Update(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, p)
}

// DELETE /products/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "product deleted")
}

// POST /products/:id/stock
func (h *Handler) AdjustStock(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcProduct.AdjustStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.service.AdjustStock(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, p)
}

// POST /products/:id/image  — multipart/form-data, field: "image"
func (h *Handler) UploadImage(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		response.BadRequest(c, "image file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "failed to read file")
		return
	}
	result, err := h.upload.UploadProductImage(id, data, header.Filename)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.UpdateImage(shared.MustBusinessID(c), id, result.URL); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"image_url": result.URL})
}
