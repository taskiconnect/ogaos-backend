// internal/worker/assessment.go
package worker

import (
	"log"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

// AssessmentWorker expires assessment sessions that were never started or completed.
// Run every hour (or more frequently) via a ticker.
type AssessmentWorker struct {
	db *gorm.DB
}

func NewAssessmentWorker(db *gorm.DB) *AssessmentWorker {
	return &AssessmentWorker{db: db}
}

// Run expires all assessment sessions past their ExpiresAt timestamp.
// For each expired session it also updates the linked application's assessment_status.
func (w *AssessmentWorker) Run() {
	log.Println("[ASSESSMENT] Running expiry check")

	var sessions []models.AssessmentSession
	w.db.Where(
		"status IN ? AND expires_at < ?",
		[]string{models.SessionStatusPending, models.SessionStatusInProgress},
		time.Now(),
	).Find(&sessions)

	log.Printf("[ASSESSMENT] Expiring %d sessions", len(sessions))

	for _, session := range sessions {
		// Mark session as expired
		w.db.Model(&session).Update("status", models.SessionStatusExpired)

		// Mark linked application's assessment_status as expired
		w.db.Model(&models.RecruitmentApplication{}).
			Where("id = ? AND assessment_status IN ?",
				session.ApplicationID,
				[]string{models.AssessmentStatusPending, models.AssessmentStatusInProgress},
			).
			Update("assessment_status", models.AssessmentStatusExpired)
	}

	log.Println("[ASSESSMENT] Expiry check complete")
}
