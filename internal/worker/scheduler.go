// internal/worker/scheduler.go
package worker

import (
	"log"
	"time"

	"gorm.io/gorm"

	pkgPaystack "ogaos-backend/internal/external/paystack"
)

// Scheduler holds all workers and runs them on fixed intervals.
type Scheduler struct {
	payout       *PayoutWorker
	subscription *SubscriptionWorker
	overdue      *OverdueWorker
	assessment   *AssessmentWorker
}

func NewScheduler(db *gorm.DB, paystackClient *pkgPaystack.Client) *Scheduler {
	return &Scheduler{
		payout:       NewPayoutWorker(db, paystackClient),
		subscription: NewSubscriptionWorker(db),
		overdue:      NewOverdueWorker(db),
		assessment:   NewAssessmentWorker(db),
	}
}

// Start launches all workers in background goroutines.
// Call once from main() after the DB and router are initialised.
// Pass a done channel to stop all workers cleanly on shutdown.
func (s *Scheduler) Start(done <-chan struct{}) {
	log.Println("[SCHEDULER] Starting background workers")

	go s.loop("PAYOUT", 10*time.Minute, s.payout.Run, done)
	go s.loop("SUBSCRIPTION", 24*time.Hour, s.subscription.RunExpiry, done)
	go s.loop("REMINDER", 24*time.Hour, s.subscription.RunReminders, done)
	go s.loop("OVERDUE", 24*time.Hour, s.overdue.Run, done)
	go s.loop("ASSESSMENT", 1*time.Hour, s.assessment.Run, done)
}

// PayoutWorker exposes the payout worker so webhook handlers can call
// MarkPayoutComplete / MarkPayoutFailed directly.
func (s *Scheduler) Payout() *PayoutWorker {
	return s.payout
}

// ─── internal ─────────────────────────────────────────────────────────────────

func (s *Scheduler) loop(name string, interval time.Duration, fn func(), done <-chan struct{}) {
	// Fire immediately on first tick, then on interval
	log.Printf("[SCHEDULER] %s worker started (interval: %s)", name, interval)
	fn()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fn()
		case <-done:
			log.Printf("[SCHEDULER] %s worker stopping", name)
			return
		}
	}
}
