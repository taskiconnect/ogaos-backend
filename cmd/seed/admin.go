// cmd/seed/admin.go
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func SeedAdmin(db *gorm.DB) {
	const adminEmail = "ogaostaski@gmail.com"

	// Check if the admin already exists
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM platform_admins WHERE LOWER(email) = LOWER(?)`,
		adminEmail,
	).Scan(&count).Error; err != nil {
		log.Fatalf("[seed] Failed to check existing admin: %v", err)
	}

	if count > 0 {
		log.Println("[seed] Platform admin already exists — skipping")
		return
	}

	token, tokenHash, err := generateSeedToken()
	if err != nil {
		log.Fatalf("[seed] Failed to generate setup token: %v", err)
	}

	id := uuid.New()
	expiresAt := time.Now().Add(24 * time.Hour)

	// Use a raw INSERT to avoid GORM AutoMigrate / constraint inference entirely.
	// Column names match exactly what your SQL migrations created.
	if err := db.Exec(`
		INSERT INTO platform_admins (
			id,
			email,
			first_name,
			last_name,
			password_hash,
			role,
			is_active,
			password_set_at,
			password_reset_token,
			reset_token_expires,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		id,
		adminEmail,
		"Miracle", // ← update before running in production
		"Olapade", // ← update before running in production
		"UNSET",   // can never be produced by bcrypt — blocks login until SetupPassword is called
		"super_admin",
		true,
		nil, // password_set_at — NULL until they set a password
		tokenHash,
		expiresAt,
	).Error; err != nil {
		log.Fatalf("[seed] Failed to create platform admin: %v", err)
	}

	log.Println("[seed] ✓ Platform super_admin created")
	log.Printf("[seed]   Email     : %s", adminEmail)
	log.Printf("[seed]   Name      : Oga Admin")
	log.Printf("[seed]   Role      : super_admin")
	log.Printf("[seed]   Token     : %s", token)
	log.Printf("[seed]   Setup URL : <YOUR_FRONTEND_URL>/admin/setup-password?token=%s", token)
	log.Println("[seed]   ⚠  Save the token above — it will not be shown again.")
}

// generateSeedToken returns (rawToken, storedHash).
// The raw token is printed to stdout; only the SHA-256 hash is stored in the DB.
func generateSeedToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand.Read: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = base64.StdEncoding.EncodeToString(h[:])
	return raw, hash, nil
}
