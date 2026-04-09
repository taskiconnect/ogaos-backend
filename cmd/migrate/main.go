package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	var (
		direction = flag.String("direction", "up", "migration direction: up, down, or force")
		steps     = flag.Int("steps", 0, "number of migrations to apply (0 = all); for force: the version number to set")
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

	// Add timeout parameters to the URL to override Leapcell's default timeouts
	dbURL = addTimeoutParameters(dbURL)

	// Open the connection with retry logic
	var sqlDB *sql.DB
	var err error

	for i := 0; i < 3; i++ {
		sqlDB, err = sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("attempt %d: failed to open database: %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Test the connection
		if err = sqlDB.Ping(); err != nil {
			log.Printf("attempt %d: failed to ping database: %v", i+1, err)
			sqlDB.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		break
	}

	if err != nil {
		log.Fatalf("failed to connect to database after 3 attempts: %v", err)
	}
	defer sqlDB.Close()

	// Set session parameters to disable timeouts
	if _, err := sqlDB.Exec("SET statement_timeout = 0"); err != nil {
		log.Printf("warning: failed to set statement_timeout: %v", err)
	}
	if _, err := sqlDB.Exec("SET lock_timeout = 0"); err != nil {
		log.Printf("warning: failed to set lock_timeout: %v", err)
	}
	if _, err := sqlDB.Exec("SET idle_in_transaction_session_timeout = 0"); err != nil {
		log.Printf("warning: failed to set idle_in_transaction_session_timeout: %v", err)
	}

	// Configure the postgres driver with custom settings
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{
		MigrationsTable:       "schema_migrations",
		StatementTimeout:      0, // Disable statement timeout
		MultiStatementEnabled: false,
		DatabaseName:          getDatabaseName(dbURL),
	})
	if err != nil {
		log.Fatalf("failed to create migrate driver: %v", err)
	}

	const migrationsPath = "file://migrations"

	m, err := migrate.NewWithDatabaseInstance(migrationsPath, "postgres", driver)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}
	defer m.Close()

	// Run the migration based on direction
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
	case "force":
		if *steps == 0 {
			log.Fatal("force requires a version number, e.g.: -direction force -steps 13")
		}
		err = m.Force(*steps)
		if err != nil {
			log.Fatalf("force failed: %v", err)
		}
		fmt.Printf("Database forced to version %d\n", *steps)
		return
	default:
		log.Fatalf("unknown direction: %s (use up, down, or force)", *direction)
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migration failed: %v", err)
	}

	if err == migrate.ErrNoChange {
		fmt.Println("No changes to apply")
	} else {
		fmt.Println("Migration completed successfully")
	}
}

// addTimeoutParameters adds statement_timeout and lock_timeout parameters to the database URL
func addTimeoutParameters(dbURL string) string {
	// Check if URL already has query parameters
	separator := "?"
	if strings.Contains(dbURL, "?") {
		separator = "&"
	}

	// Add timeout parameters
	return fmt.Sprintf("%s%sstatement_timeout=0&lock_timeout=0", dbURL, separator)
}

// getDatabaseName extracts the database name from the connection URL
func getDatabaseName(dbURL string) string {
	// Find the last slash before query parameters
	pathStart := strings.Index(dbURL, "/")
	if pathStart == -1 {
		return ""
	}

	pathStart = strings.Index(dbURL[pathStart+1:], "/")
	if pathStart == -1 {
		return ""
	}

	pathStart += strings.Index(dbURL, "/") + 1

	pathEnd := strings.Index(dbURL[pathStart:], "?")
	if pathEnd == -1 {
		pathEnd = len(dbURL)
	} else {
		pathEnd += pathStart
	}

	return dbURL[pathStart:pathEnd]
}
