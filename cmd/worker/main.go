// cmd/worker/main.go
package main

import (
	"log"

	"ogaos-backend/internal/config"
	// "ogaos-backend/internal/worker" // future package
)

func main() {
	cfg := config.Get()
	log.Printf("Starting OgaOs Worker (%s mode)", cfg.Env)

	// Example: run background jobs, queue processor, cron, etc.
	// worker.StartEmailQueue()
	// worker.StartDebtReminderCron()
	// worker.StartNotificationWorker()

	// Block forever
	select {}
}
