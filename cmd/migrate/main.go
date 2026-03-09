package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	var (
		direction = flag.String("direction", "up", "migration direction: up or down")
		steps     = flag.Int("steps", 0, "number of migrations to apply (0 = all)")
	)

	flag.Parse()

	// Load .env file if present (silent if missing)
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set.\n" +
			"→ Add it to your .env file, e.g.:\n" +
			"  DATABASE_URL=postgres://user:password@localhost:5432/ogaos?sslmode=disable")
	}

	const migrationsPath = "file://migrations"

	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}
	defer m.Close()

	switch *direction {
	case "up":
		if *steps == 0 {
			err = m.Up()
		} else {
			err = m.Steps(*steps)
		}
	case "down":
		if *steps == 0 {
			err = m.Down()
		} else {
			err = m.Steps(-*steps)
		}
	default:
		log.Fatalf("unknown direction: %s (use up or down)", *direction)
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migration failed: %v", err)
	}

	fmt.Println("Migration completed successfully")
}
