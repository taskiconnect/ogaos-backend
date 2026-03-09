// internal/domain/models/assessment_question.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	DifficultyEasy   = "easy"
	DifficultyMedium = "medium"
	DifficultyHard   = "hard"
)

type AssessmentQuestion struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Category      string    `gorm:"size:50;not null;index" json:"category"` // general | customer_service | sales | admin | tech
	QuestionText  string    `gorm:"type:text;not null" json:"question_text"`
	OptionA       string    `gorm:"size:500;not null" json:"option_a"`
	OptionB       string    `gorm:"size:500;not null" json:"option_b"`
	OptionC       string    `gorm:"size:500;not null" json:"option_c"`
	OptionD       string    `gorm:"size:500;not null" json:"option_d"`
	CorrectOption string    `gorm:"size:1;not null" json:"correct_option"` // "A" | "B" | "C" | "D"
	Explanation   *string   `gorm:"type:text" json:"explanation"`          // optional — not shown to applicant
	Difficulty    string    `gorm:"size:10;not null;default:'medium'" json:"difficulty"`
	IsActive      bool      `gorm:"default:true;index" json:"is_active"`
	CreatedBy     uuid.UUID `gorm:"type:uuid;not null" json:"created_by"` // platform_admin user_id
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
