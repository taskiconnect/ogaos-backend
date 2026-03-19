// internal/api/handlers/business/handler.go
package business

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcBusiness "ogaos-backend/internal/service/business"
	svcUpload "ogaos-backend/internal/service/upload"
)

type Handler struct {
	service *svcBusiness.Service
	upload  *svcUpload.Service
}

func NewHandler(service *svcBusiness.Service, upload *svcUpload.Service) *Handler {
	return &Handler{service: service, upload: upload}
}

// GET /business/me
func (h *Handler) Get(c *gin.Context) {
	b, err := h.service.Get(shared.MustBusinessID(c))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, b)
}

// PATCH /business/me
func (h *Handler) Update(c *gin.Context) {
	var req svcBusiness.UpdateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	b, err := h.service.Update(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, b)
}

// POST /business/me/logo  — multipart/form-data, field: "logo"
func (h *Handler) UploadLogo(c *gin.Context) {
	businessID := shared.MustBusinessID(c)
	file, header, err := c.Request.FormFile("logo")
	if err != nil {
		response.BadRequest(c, "logo file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "failed to read file")
		return
	}
	result, err := h.upload.UploadLogo(businessID, data, header.Filename)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.UpdateLogo(businessID, result.URL); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"logo_url": result.URL})
}

// PATCH /business/me/visibility
func (h *Handler) SetVisibility(c *gin.Context) {
	var req struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.SetProfilePublic(shared.MustBusinessID(c), req.IsPublic); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "visibility updated")
}

// POST /business/me/gallery  — multipart/form-data, field: "image"
func (h *Handler) AddGalleryImage(c *gin.Context) {
	businessID := shared.MustBusinessID(c)
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
	result, err := h.upload.UploadBusinessGalleryImage(businessID, data, header.Filename)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	b, err := h.service.AddGalleryImage(businessID, result.URL)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, b)
}

// DELETE /business/me/gallery/:index
func (h *Handler) RemoveGalleryImage(c *gin.Context) {
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		response.BadRequest(c, "invalid gallery index")
		return
	}
	b, err := h.service.RemoveGalleryImage(shared.MustBusinessID(c), index)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, b)
}

// PATCH /business/me/storefront-video
// Body: { "video_url": "https://youtube.com/..." }  — pass empty string to clear
func (h *Handler) SetStorefrontVideo(c *gin.Context) {
	var req struct {
		VideoURL string `json:"video_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	b, err := h.service.SetStorefrontVideo(shared.MustBusinessID(c), req.VideoURL)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, b)
}

// GET /public/business/:slug  — no auth required
func (h *Handler) GetPublicProfile(c *gin.Context) {
	b, err := h.service.GetPublicProfile(c.Param("slug"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, b)
}
