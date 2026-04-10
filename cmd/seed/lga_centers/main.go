package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"ogaos-backend/internal/config"
)

type LGA struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	StateName   string  `json:"state_name"`
	CountryName string  `json:"country_name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

func main() {
	logger := log.New(os.Stdout, "[seed_lga_centers] ", log.LstdFlags|log.Lshortfile)

	// ✅ Load your existing config (this loads .env automatically)
	cfg := config.Load()

	// ✅ Get DB URL from config
	db, err := sql.Open("pgx", cfg.DBURL)
	if err != nil {
		logger.Fatalf("failed to connect DB: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// ✅ JSON path
	jsonPath := getJSONPath()

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		logger.Fatalf("failed to read JSON: %v", err)
	}

	var lgas []LGA
	if err := json.Unmarshal(data, &lgas); err != nil {
		logger.Fatalf("failed to parse JSON: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Fatalf("failed to start transaction: %v", err)
	}

	inserted := 0
	updated := 0
	skipped := 0

	query := `
	INSERT INTO local_government_centers (
		country,
		state,
		local_government,
		latitude,
		longitude,
		created_at,
		updated_at
	)
	VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	ON CONFLICT (country, state, local_government)
	DO UPDATE SET
		latitude = EXCLUDED.latitude,
		longitude = EXCLUDED.longitude,
		updated_at = NOW()
	RETURNING xmax = 0;
	`

	for i, lga := range lgas {
		country := normalize(lga.CountryName)
		state := normalize(lga.StateName)
		name := normalize(lga.Name)

		if country == "" || state == "" || name == "" {
			skipped++
			continue
		}

		if lga.Latitude == 0 && lga.Longitude == 0 {
			skipped++
			continue
		}

		var isInsert bool
		err := tx.QueryRowContext(
			ctx,
			query,
			country,
			state,
			name,
			lga.Latitude,
			lga.Longitude,
		).Scan(&isInsert)

		if err != nil {
			_ = tx.Rollback()
			logger.Fatalf("failed at row %d: %v", i, err)
		}

		if isInsert {
			inserted++
		} else {
			updated++
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Fatalf("commit failed: %v", err)
	}

	logger.Printf("DONE ✅ inserted=%d updated=%d skipped=%d", inserted, updated, skipped)
}

func getJSONPath() string {
	path := os.Getenv("LGA_JSON_PATH")
	if path != "" {
		return path
	}
	return "data/lgas.json"
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}
