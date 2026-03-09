// internal/worker/overdue.go
package worker

import (
	"log"
	"time"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

// OverdueWorker marks invoices and debts as overdue when their due dates pass.
// Run daily via a ticker or cron.
type OverdueWorker struct {
	db *gorm.DB
}

func NewOverdueWorker(db *gorm.DB) *OverdueWorker {
	return &OverdueWorker{db: db}
}

// RunInvoices marks all sent invoices past their due_date as overdue.
func (w *OverdueWorker) RunInvoices() {
	result := w.db.Model(&models.Invoice{}).
		Where("status = ? AND due_date < ?", models.InvoiceStatusSent, time.Now()).
		Update("status", models.InvoiceStatusOverdue)

	log.Printf("[OVERDUE] Marked %d invoices as overdue", result.RowsAffected)
}

// RunDebts marks outstanding/partial debts past their due_date as overdue.
func (w *OverdueWorker) RunDebts() {
	result := w.db.Model(&models.Debt{}).
		Where("due_date < ? AND status NOT IN ?",
			time.Now(),
			[]string{models.DebtStatusSettled, models.DebtStatusOverdue},
		).
		Update("status", models.DebtStatusOverdue)

	log.Printf("[OVERDUE] Marked %d debts as overdue", result.RowsAffected)
}

// Run executes all overdue checks in sequence.
func (w *OverdueWorker) Run() {
	log.Println("[OVERDUE] Running overdue checks")
	w.RunInvoices()
	w.RunDebts()
	log.Println("[OVERDUE] Overdue checks complete")
}
