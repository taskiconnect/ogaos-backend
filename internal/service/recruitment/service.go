package recruitment

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/cursor"
	"ogaos-backend/internal/pkg/email"
)

type Service struct {
	db          *gorm.DB
	frontendURL string
}

func NewService(db *gorm.DB, frontendURL string) *Service {
	return &Service{db: db, frontendURL: frontendURL}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type CreateJobRequest struct {
	Title               string  `json:"title" binding:"required"`
	Description         string  `json:"description" binding:"required"`
	Requirements        *string `json:"requirements"`
	Responsibilities    *string `json:"responsibilities"`
	Type                string  `json:"type" binding:"required"`
	Location            *string `json:"location"`
	IsRemote            bool    `json:"is_remote"`
	SalaryRangeMin      *int64  `json:"salary_range_min"`
	SalaryRangeMax      *int64  `json:"salary_range_max"`
	ApplicationDeadline *string `json:"application_deadline"`
	AssessmentEnabled   bool    `json:"assessment_enabled"`
	AssessmentCategory  *string `json:"assessment_category"`
	PassThreshold       int     `json:"pass_threshold"`
	TimeLimitMinutes    int     `json:"time_limit_minutes"`
}

type ApplyRequest struct {
	FirstName   string  `json:"first_name" binding:"required"`
	LastName    string  `json:"last_name" binding:"required"`
	Email       string  `json:"email" binding:"required,email"`
	PhoneNumber string  `json:"phone_number" binding:"required"`
	CoverLetter *string `json:"cover_letter"`
}

type ReviewRequest struct {
	Status      string  `json:"status" binding:"required"`
	ReviewNotes *string `json:"review_notes"`
}

type JobListFilter struct {
	Status string
	Type   string
	Cursor string
	Limit  int
}

type AppListFilter struct {
	JobOpeningID *uuid.UUID
	Status       string
	Cursor       string
	Limit        int
}

type PublicJobListFilter struct {
	Query    string
	Type     string
	Location string
	IsRemote *bool
	Cursor   string
	Limit    int
}

type PublicJobItem struct {
	ID                  uuid.UUID  `json:"id"`
	BusinessID          uuid.UUID  `json:"business_id"`
	BusinessName        string     `json:"business_name"`
	BusinessSlug        string     `json:"business_slug"`
	BusinessLogoURL     *string    `json:"business_logo_url"`
	Title               string     `json:"title"`
	Slug                string     `json:"slug"`
	Description         string     `json:"description"`
	Requirements        *string    `json:"requirements,omitempty"`
	Responsibilities    *string    `json:"responsibilities,omitempty"`
	Type                string     `json:"type"`
	Location            *string    `json:"location,omitempty"`
	IsRemote            bool       `json:"is_remote"`
	SalaryRangeMin      *int64     `json:"salary_range_min,omitempty"`
	SalaryRangeMax      *int64     `json:"salary_range_max,omitempty"`
	ApplicationDeadline *time.Time `json:"application_deadline,omitempty"`
	AssessmentEnabled   bool       `json:"assessment_enabled"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func parseApplicationDeadline(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}

	v := strings.TrimSpace(*value)
	if v == "" {
		return nil, nil
	}

	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			parsed := t
			return &parsed, nil
		}
	}

	return nil, errors.New("application_deadline must be in YYYY-MM-DD or RFC3339 format")
}

// ─── Job Openings ─────────────────────────────────────────────────────────────

func (s *Service) CreateJob(businessID, postedBy uuid.UUID, req CreateJobRequest) (*models.JobOpening, error) {
	threshold := req.PassThreshold
	if threshold == 0 {
		threshold = 60
	}

	timeLimit := req.TimeLimitMinutes
	if timeLimit == 0 {
		timeLimit = 30
	}

	deadline, err := parseApplicationDeadline(req.ApplicationDeadline)
	if err != nil {
		return nil, err
	}

	job := models.JobOpening{
		BusinessID:          businessID,
		PostedBy:            postedBy,
		Title:               req.Title,
		Slug:                s.generateJobSlug(businessID, req.Title),
		Description:         req.Description,
		Requirements:        req.Requirements,
		Responsibilities:    req.Responsibilities,
		Type:                req.Type,
		Location:            req.Location,
		IsRemote:            req.IsRemote,
		SalaryRangeMin:      req.SalaryRangeMin,
		SalaryRangeMax:      req.SalaryRangeMax,
		ApplicationDeadline: deadline,
		Status:              models.JobStatusOpen,
		AssessmentEnabled:   req.AssessmentEnabled,
		AssessmentCategory:  req.AssessmentCategory,
		PassThreshold:       threshold,
		TimeLimitMinutes:    timeLimit,
	}

	if err := s.db.Create(&job).Error; err != nil {
		return nil, err
	}

	return &job, nil
}

func (s *Service) GetJob(businessID, jobID uuid.UUID) (*models.JobOpening, error) {
	var job models.JobOpening
	if err := s.db.Where("id = ? AND business_id = ?", jobID, businessID).First(&job).Error; err != nil {
		return nil, errors.New("job opening not found")
	}
	return &job, nil
}

func (s *Service) GetPublicJob(slug string) (*models.JobOpening, error) {
	var job models.JobOpening
	if err := s.db.Where("slug = ? AND status = ?", slug, models.JobStatusOpen).First(&job).Error; err != nil {
		return nil, errors.New("job opening not found")
	}

	if job.ApplicationDeadline != nil && time.Now().After(*job.ApplicationDeadline) {
		return nil, errors.New("job opening not found")
	}

	return &job, nil
}

func (s *Service) ListJobs(businessID uuid.UUID, filter JobListFilter) ([]models.JobOpening, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	q := s.db.Model(&models.JobOpening{}).Where("business_id = ?", businessID)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cur, id)
	}

	var jobs []models.JobOpening
	if err := q.Order("created_at DESC, id DESC").Limit(filter.Limit + 1).Find(&jobs).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(jobs) > filter.Limit {
		last := jobs[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		jobs = jobs[:filter.Limit]
	}

	return jobs, nextCursor, nil
}

func (s *Service) ListPublicJobs(filter PublicJobListFilter) ([]PublicJobItem, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	now := time.Now()

	type publicJobRow struct {
		ID                  uuid.UUID
		BusinessID          uuid.UUID
		BusinessName        string
		BusinessSlug        string
		BusinessLogoURL     *string
		Title               string
		Slug                string
		Description         string
		Requirements        *string
		Responsibilities    *string
		Type                string
		Location            *string
		IsRemote            bool
		SalaryRangeMin      *int64
		SalaryRangeMax      *int64
		ApplicationDeadline *time.Time
		AssessmentEnabled   bool
		CreatedAt           time.Time
	}

	q := s.db.Table("job_openings AS j").
		Select(`
			j.id,
			j.business_id,
			b.name AS business_name,
			b.slug AS business_slug,
			b.logo_url AS business_logo_url,
			j.title,
			j.slug,
			j.description,
			j.requirements,
			j.responsibilities,
			j.type,
			j.location,
			j.is_remote,
			j.salary_range_min,
			j.salary_range_max,
			j.application_deadline,
			j.assessment_enabled,
			j.created_at
		`).
		Joins("JOIN businesses b ON b.id = j.business_id").
		Where("j.status = ?", models.JobStatusOpen).
		Where("(j.application_deadline IS NULL OR j.application_deadline >= ?)", now)

	if filter.Query != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		q = q.Where(`
			LOWER(j.title) LIKE ?
			OR LOWER(j.description) LIKE ?
			OR LOWER(COALESCE(j.type, '')) LIKE ?
			OR LOWER(COALESCE(j.location, '')) LIKE ?
			OR LOWER(b.name) LIKE ?
		`, like, like, like, like, like)
	}

	if filter.Type != "" {
		q = q.Where("j.type = ?", filter.Type)
	}

	if filter.Location != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(filter.Location)) + "%"
		q = q.Where("LOWER(COALESCE(j.location, '')) LIKE ?", like)
	}

	if filter.IsRemote != nil {
		q = q.Where("j.is_remote = ?", *filter.IsRemote)
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(j.created_at, j.id) < (?, ?)", cur, id)
	}

	var rows []publicJobRow
	if err := q.Order("j.created_at DESC, j.id DESC").Limit(filter.Limit + 1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(rows) > filter.Limit {
		last := rows[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		rows = rows[:filter.Limit]
	}

	items := make([]PublicJobItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PublicJobItem{
			ID:                  row.ID,
			BusinessID:          row.BusinessID,
			BusinessName:        row.BusinessName,
			BusinessSlug:        row.BusinessSlug,
			BusinessLogoURL:     row.BusinessLogoURL,
			Title:               row.Title,
			Slug:                row.Slug,
			Description:         row.Description,
			Requirements:        row.Requirements,
			Responsibilities:    row.Responsibilities,
			Type:                row.Type,
			Location:            row.Location,
			IsRemote:            row.IsRemote,
			SalaryRangeMin:      row.SalaryRangeMin,
			SalaryRangeMax:      row.SalaryRangeMax,
			ApplicationDeadline: row.ApplicationDeadline,
			AssessmentEnabled:   row.AssessmentEnabled,
			CreatedAt:           row.CreatedAt,
		})
	}

	return items, nextCursor, nil
}

func (s *Service) CloseJob(businessID, jobID uuid.UUID) error {
	result := s.db.Model(&models.JobOpening{}).
		Where("id = ? AND business_id = ?", jobID, businessID).
		Update("status", models.JobStatusClosed)

	if result.RowsAffected == 0 {
		return errors.New("job opening not found")
	}

	return result.Error
}

// ─── Applications ─────────────────────────────────────────────────────────────

func (s *Service) Apply(jobID uuid.UUID, req ApplyRequest, cvURL *string) (*models.RecruitmentApplication, error) {
	var job models.JobOpening
	if err := s.db.First(&job, jobID).Error; err != nil {
		return nil, errors.New("job not found")
	}

	if job.Status != models.JobStatusOpen {
		return nil, errors.New("this job is no longer accepting applications")
	}

	if job.ApplicationDeadline != nil && time.Now().After(*job.ApplicationDeadline) {
		return nil, errors.New("the application deadline has passed")
	}

	var count int64
	s.db.Model(&models.RecruitmentApplication{}).
		Where("job_opening_id = ? AND email = ?", jobID, req.Email).
		Count(&count)

	if count > 0 {
		return nil, errors.New("you have already applied for this position")
	}

	assessmentStatus := models.AssessmentStatusNotRequired
	if job.AssessmentEnabled {
		assessmentStatus = models.AssessmentStatusPending
	}

	app := models.RecruitmentApplication{
		BusinessID:       job.BusinessID,
		JobOpeningID:     jobID,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		Email:            req.Email,
		PhoneNumber:      req.PhoneNumber,
		CoverLetter:      req.CoverLetter,
		CVUrl:            cvURL,
		Status:           models.ApplicationStatusNew,
		AssessmentStatus: assessmentStatus,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&app).Error; err != nil {
			return err
		}

		return tx.Model(&models.JobOpening{}).
			Where("id = ?", jobID).
			UpdateColumn("application_count", gorm.Expr("application_count + 1")).
			Error
	}); err != nil {
		return nil, err
	}

	if job.AssessmentEnabled {
		assessmentURL := fmt.Sprintf("%s/assessment/%s", s.frontendURL, app.ID)
		email.SendAssessmentLink(
			req.Email,
			req.FirstName+" "+req.LastName,
			job.Title,
			"",
			assessmentURL,
			job.TimeLimitMinutes,
		)
	}

	return &app, nil
}

func (s *Service) ListApplications(businessID uuid.UUID, filter AppListFilter) ([]models.RecruitmentApplication, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	q := s.db.Model(&models.RecruitmentApplication{}).Where("business_id = ?", businessID)
	if filter.JobOpeningID != nil {
		q = q.Where("job_opening_id = ?", *filter.JobOpeningID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}

	if filter.Cursor != "" {
		cur, id, err := cursor.Decode(filter.Cursor)
		if err != nil {
			return nil, "", errors.New("invalid cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", cur, id)
	}

	var apps []models.RecruitmentApplication
	if err := q.Preload("JobOpening").Order("created_at DESC, id DESC").Limit(filter.Limit + 1).Find(&apps).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(apps) > filter.Limit {
		last := apps[filter.Limit-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
		apps = apps[:filter.Limit]
	}

	return apps, nextCursor, nil
}

func (s *Service) ReviewApplication(businessID, appID uuid.UUID, req ReviewRequest) (*models.RecruitmentApplication, error) {
	validStatuses := map[string]bool{
		models.ApplicationStatusReviewing:   true,
		models.ApplicationStatusShortlisted: true,
		models.ApplicationStatusRejected:    true,
		models.ApplicationStatusHired:       true,
	}

	if !validStatuses[req.Status] {
		return nil, errors.New("invalid status")
	}

	var app models.RecruitmentApplication
	if err := s.db.Where("id = ? AND business_id = ?", appID, businessID).First(&app).Error; err != nil {
		return nil, errors.New("application not found")
	}

	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.ReviewNotes != nil {
		updates["review_notes"] = *req.ReviewNotes
	}

	s.db.Model(&app).Updates(updates)
	return &app, nil
}

func (s *Service) SubmitAssessment(appID uuid.UUID, score int) error {
	var app models.RecruitmentApplication
	if err := s.db.Preload("JobOpening").First(&app, appID).Error; err != nil {
		return errors.New("application not found")
	}

	if app.AssessmentStatus != models.AssessmentStatusInProgress {
		return errors.New("assessment is not in progress")
	}

	now := time.Now()
	passed := score >= app.JobOpening.PassThreshold

	updates := map[string]interface{}{
		"assessment_score":        score,
		"assessment_status":       models.AssessmentStatusCompleted,
		"assessment_completed_at": now,
	}

	s.db.Model(&app).Updates(updates)

	email.SendAssessmentResult(app.Email, app.FirstName+" "+app.LastName, app.JobOpening.Title, "", passed)
	return nil
}

func (s *Service) generateJobSlug(businessID uuid.UUID, title string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, title)

	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	slug = strings.Trim(slug, "-")
	return fmt.Sprintf("%s-%s", slug, businessID.String()[:8])
}
