// cmd/worker/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ogaos-backend/internal/config"
	"ogaos-backend/internal/db"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	"ogaos-backend/internal/worker"
)

func main() {
	// Load configuration
	cfg := config.Get()

	log.Printf("Starting OgaOs Worker (%s mode)", cfg.Env)

	// Initialize database connection
	db.InitDB()
	log.Println("Database connected successfully")

	// Initialize external clients
	paystackClient := pkgPaystack.NewClient(cfg.PaystackSecretKey)

	// Initialize the scheduler (which runs all background workers)
	scheduler := worker.NewScheduler(db.DB, paystackClient)

	// Channel to signal workers to stop
	workerDone := make(chan struct{})

	// Start all background workers (payouts, subscription expiry, reminders, etc.)
	scheduler.Start(workerDone)

	log.Println("All background workers started successfully")

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit // Wait for shutdown signal

	log.Println("Shutting down worker...")

	// Signal all workers to stop
	close(workerDone)

	// Give workers some time to finish current tasks
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait a bit for clean shutdown (you can add more graceful logic if needed)
	<-ctx.Done()

	log.Println("Worker exited gracefully")
}
