// internal/api/handlers/digital/handler.go
package digital

import (
	"io"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcDigital "ogaos-backend/internal/service/digital"
	svcUpload "ogaos-backend/internal/service/upload"
)

type Handler struct {
	service *svcDigital.Service
	upload  *svcUpload.Service
}

func NewHandler(s *svcDigital.Service, u *svcUpload.Service) *Handler {
	return &Handler{service: s, upload: u}
}

// POST /digital-products
func (h *Handler) Create(c *gin.Context) {
	var req svcDigital.CreateRequest
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

// GET /digital-products
func (h *Handler) List(c *gin.Context) {
	page, limit := shared.Paginate(c)
	products, total, err := h.service.List(shared.MustBusinessID(c), page, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.List(c, products, total, page, limit)
}

// GET /digital-products/:id
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

// PATCH /digital-products/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcDigital.UpdateRequest
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

// DELETE /digital-products/:id
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

// POST /digital-products/:id/file  — multipart, field: "file"
func (h *Handler) UploadFile(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "failed to read file")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	result, err := h.upload.UploadDigitalProductFile(id, data, header.Filename, mimeType)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.AttachFile(shared.MustBusinessID(c), id, result.URL, result.FileSize, result.MimeType); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "file uploaded successfully")
}

// POST /digital-products/:id/cover  — multipart, field: "cover"
func (h *Handler) UploadCover(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	file, header, err := c.Request.FormFile("cover")
	if err != nil {
		response.BadRequest(c, "cover image is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "failed to read file")
		return
	}
	result, err := h.upload.UploadCoverImage(id, data, header.Filename)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.AttachCoverImage(shared.MustBusinessID(c), id, result.URL); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"cover_image_url": result.URL})
}

// ─── Public storefront ────────────────────────────────────────────────────────

// GET /public/store/:slug
func (h *Handler) GetPublicProduct(c *gin.Context) {
	p, err := h.service.GetPublic(c.Param("slug"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, p)
}

// POST /public/store/:id/purchase
func (h *Handler) Purchase(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcDigital.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	order, err := h.service.CompletePurchase(id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, gin.H{
		"message":  "Purchase complete. Check your email for the download link.",
		"order_id": order.ID,
	})
}

// GET /public/orders/:order_id/download?email=buyer@example.com
func (h *Handler) GetDownload(c *gin.Context) {
	orderID, ok := shared.ParseID(c, "order_id")
	if !ok {
		return
	}
	buyerEmail := c.Query("email")
	if buyerEmail == "" {
		response.BadRequest(c, "email query param is required")
		return
	}
	url, err := h.service.GetDownloadURL(orderID, buyerEmail)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, gin.H{"download_url": url})
}
