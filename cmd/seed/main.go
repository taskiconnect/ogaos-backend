// cmd/seed/main.go
package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Load .env if present — safe to ignore in production
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("[seed] DATABASE_URL env var is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Silence the slow-query warnings — expected on a remote DB
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("[seed] Failed to connect to database: %v", err)
	}

	// Verify the table exists — if not, migrations haven't been run yet
	var exists bool
	row := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = CURRENT_SCHEMA()
			AND table_name = 'platform_admins'
		)
	`).Row()
	if err := row.Scan(&exists); err != nil || !exists {
		log.Fatal("[seed] platform_admins table does not exist — run your SQL migrations first")
	}

	SeedAdmin(db)
}
