// internal/domain/models/assessment_session.go
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	SessionStatusPending    = "pending"
	SessionStatusInProgress = "in_progress"
	SessionStatusCompleted  = "completed"
	SessionStatusExpired    = "expired"
)

// QuestionSnapshot is the structure stored in the JSONB questions_snapshot
type QuestionSnapshot struct {
	ID           string `json:"id"`
	QuestionText string `json:"question_text"`
	OptionA      string `json:"option_a"`
	OptionB      string `json:"option_b"`
	OptionC      string `json:"option_c"`
	OptionD      string `json:"option_d"`
	// CorrectOption is intentionally excluded from snapshot
	// It is only used server-side for scoring
}

// AnswerSnapshot records the applicant's answer to each question
type AnswerSnapshot struct {
	QuestionID     string `json:"question_id"`
	SelectedOption string `json:"selected_option"` // "A" | "B" | "C" | "D"
}

type AssessmentSession struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ApplicationID     uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex" json:"application_id"`
	BusinessID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"business_id"`
	Token             string          `gorm:"size:255;not null;uniqueIndex" json:"token"` // one-time UUID sent to applicant
	QuestionsSnapshot json.RawMessage `gorm:"type:jsonb;not null" json:"-"`               // snapshot at time of session creation
	AnswersSnapshot   json.RawMessage `gorm:"type:jsonb" json:"-"`                        // filled on submission
	Score             *int            `json:"score"`                                      // 0-100
	TotalQuestions    int             `gorm:"not null" json:"total_questions"`
	CorrectAnswers    *int            `json:"correct_answers"`
	TimeLimitMinutes  int             `gorm:"not null;default:30" json:"time_limit_minutes"`
	Status            string          `gorm:"size:20;not null;default:'pending'" json:"status"`
	ExpiresAt         time.Time       `gorm:"not null;index" json:"expires_at"` // 24hrs from creation
	StartedAt         *time.Time      `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at"`
	CreatedAt         time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time       `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Application RecruitmentApplication `gorm:"foreignKey:ApplicationID" json:"-"`
	Business    Business               `gorm:"foreignKey:BusinessID" json:"-"`
}

// IsExpired returns true if the session link has expired
func (s *AssessmentSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt) && s.Status != SessionStatusCompleted
}
