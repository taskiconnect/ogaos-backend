// internal/db/db.go
package db

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ogaos-backend/internal/config"
)

var DB *gorm.DB

func InitDB() {
	cfg := config.Get()

	dsn := cfg.PostgresDSN()
	if dsn == "" {
		log.Fatal("No database DSN found")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	DB = db
	log.Println("Database connected successfully")
}
