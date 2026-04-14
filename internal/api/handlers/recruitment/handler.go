package recruitment

import (
	"io"
	"strconv"
	"strings"

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
	cur, limit := shared.CursorParams(c)
	jobs, nextCursor, err := h.service.ListJobs(shared.MustBusinessID(c), svcRecruitment.JobListFilter{
		Status: c.Query("status"),
		Type:   c.Query("type"),
		Cursor: cur,
		Limit:  limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.CursorList(c, jobs, nextCursor)
}

// GET /public/jobs
func (h *Handler) ListPublicJobs(c *gin.Context) {
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	isRemotePtr := parseBoolPointer(c.Query("is_remote"))

	items, nextCursor, err := h.service.ListPublicJobs(svcRecruitment.PublicJobListFilter{
		Query:    strings.TrimSpace(c.Query("q")),
		Type:     strings.TrimSpace(c.Query("type")),
		Location: strings.TrimSpace(c.Query("location")),
		IsRemote: isRemotePtr,
		Cursor:   strings.TrimSpace(c.Query("cursor")),
		Limit:    limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.CursorList(c, items, nextCursor)
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

// GET /public/jobs/:slug
func (h *Handler) GetPublicJob(c *gin.Context) {
	job, err := h.service.GetPublicJob(c.Param("slug"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, job)
}

// POST /public/jobs/:id/apply
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

// GET /applications
func (h *Handler) ListApplications(c *gin.Context) {
	cur, limit := shared.CursorParams(c)
	apps, nextCursor, err := h.service.ListApplications(shared.MustBusinessID(c), svcRecruitment.AppListFilter{
		JobOpeningID: shared.QueryUUID(c, "job_id"),
		Status:       c.Query("status"),
		Cursor:       cur,
		Limit:        limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.CursorList(c, apps, nextCursor)
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

func parseBoolPointer(raw string) *bool {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return nil
	}

	switch v {
	case "true", "1", "yes":
		b := true
		return &b
	case "false", "0", "no":
		b := false
		return &b
	default:
		return nil
	}
}
