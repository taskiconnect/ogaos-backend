// internal/api/handlers/recruitment/handler.go
package recruitment

import (
	"io"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcRecruitment "ogaos-backend/internal/service/recruitment"
	svcUpload "ogaos-backend/internal/service/upload"
)

type Handler struct {
	service *svcRecruitment.Service
	upload  *svcUpload.Service
}

func NewHandler(s *svcRecruitment.Service, u *svcUpload.Service) *Handler {
	return &Handler{service: s, upload: u}
}

// ─── Job openings ─────────────────────────────────────────────────────────────

// POST /jobs
func (h *Handler) CreateJob(c *gin.Context) {
	var req svcRecruitment.CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	job, err := h.service.CreateJob(shared.MustBusinessID(c), shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, job)
}

// GET /jobs
func (h *Handler) ListJobs(c *gin.Context) {
	page, limit := shared.Paginate(c)
	jobs, total, err := h.service.ListJobs(shared.MustBusinessID(c), svcRecruitment.JobListFilter{
		Status: c.Query("status"),
		Type:   c.Query("type"),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.List(c, jobs, total, page, limit)
}

// GET /jobs/:id
func (h *Handler) GetJob(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	job, err := h.service.GetJob(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, job)
}

// PATCH /jobs/:id/close
func (h *Handler) CloseJob(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.CloseJob(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "job closed")
}

// ─── Public (no auth) ─────────────────────────────────────────────────────────

// GET /public/jobs/:slug
func (h *Handler) GetPublicJob(c *gin.Context) {
	job, err := h.service.GetPublicJob(c.Param("slug"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, job)
}

// POST /public/jobs/:id/apply  — multipart/form-data
func (h *Handler) Apply(c *gin.Context) {
	jobID, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	req := svcRecruitment.ApplyRequest{
		FirstName:   c.PostForm("first_name"),
		LastName:    c.PostForm("last_name"),
		Email:       c.PostForm("email"),
		PhoneNumber: c.PostForm("phone_number"),
	}
	if cover := c.PostForm("cover_letter"); cover != "" {
		req.CoverLetter = &cover
	}

	var cvURL *string
	if file, header, err := c.Request.FormFile("cv"); err == nil {
		defer file.Close()
		if data, err := io.ReadAll(file); err == nil {
			if result, err := h.upload.UploadCV(jobID, data, header.Filename); err == nil {
				cvURL = &result.URL
			} else {
				response.BadRequest(c, err.Error())
				return
			}
		}
	}

	app, err := h.service.Apply(jobID, req, cvURL)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, gin.H{
		"message": "Application submitted successfully",
		"id":      app.ID,
	})
}

// ─── Applications ─────────────────────────────────────────────────────────────

// GET /applications
func (h *Handler) ListApplications(c *gin.Context) {
	page, limit := shared.Paginate(c)
	apps, total, err := h.service.ListApplications(shared.MustBusinessID(c), svcRecruitment.AppListFilter{
		JobOpeningID: shared.QueryUUID(c, "job_id"),
		Status:       c.Query("status"),
		Page:         page,
		Limit:        limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.List(c, apps, total, page, limit)
}

// PATCH /applications/:id/review
func (h *Handler) ReviewApplication(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcRecruitment.ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	app, err := h.service.ReviewApplication(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, app)
}

// POST /public/assessment/:app_id/submit
func (h *Handler) SubmitAssessment(c *gin.Context) {
	appID, ok := shared.ParseID(c, "app_id")
	if !ok {
		return
	}
	var req struct {
		Score int `json:"score" binding:"required,min=0,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.SubmitAssessment(appID, req.Score); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "assessment submitted")
}
