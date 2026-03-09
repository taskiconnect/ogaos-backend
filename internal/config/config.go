package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port    string
	Env     string
	AppName string

	// Database (Leapcell PostgreSQL)
	DBURL string // ← REQUIRED: full connection URI from Leapcell dashboard

	// JWT
	JWTSecret        string
	JWTExpiry        time.Duration
	JWTRefreshExpiry time.Duration

	// Security
	RateLimitPerMinute int
	AllowedOrigins     []string

	// App / Frontend
	FrontendURL string

	// Email (Resend) — read directly by internal/pkg/email/email.go via os.Getenv
	// Declared here so they are validated at startup and visible to the team.
	ResendAPIKey string
	EmailFrom    string

	// Paystack
	PaystackSecretKey     string
	PaystackPublicKey     string
	PaystackWebhookSecret string

	// Flutterwave
	FlutterwaveSecretKey   string
	FlutterwavePublicKey   string
	FlutterwaveWebhookHash string

	// ImageKit
	ImageKitPublicKey   string
	ImageKitPrivateKey  string
	ImageKitURLEndpoint string

	// Platform
	PlatformFeePercent int // percentage taken from digital store sales e.g. 5
}

var (
	instance *Config
	once     sync.Once
)

func Load() *Config {
	once.Do(func() {
		// Load .env file if present (silent if missing)
		_ = godotenv.Load()

		cfg := &Config{
			Port:    getEnv("PORT", "8080"),
			Env:     getEnv("ENV", "development"),
			AppName: getEnv("APP_NAME", "OgaOs"),

			// Database
			DBURL: getEnv("DATABASE_URL", ""),

			// JWT
			JWTSecret:        getEnv("JWT_SECRET", ""),
			JWTExpiry:        getEnvDuration("JWT_EXPIRY", 15*time.Minute),
			JWTRefreshExpiry: getEnvDuration("JWT_REFRESH_EXPIRY", 168*time.Hour), // 7 days

			// Security
			RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 100),
			AllowedOrigins:     splitTrim(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),

			// App
			FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),

			// Email
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			EmailFrom:    getEnv("EMAIL_FROM", "hello@ogaos.com"),

			// Paystack
			PaystackSecretKey:     getEnv("PAYSTACK_SECRET_KEY", ""),
			PaystackPublicKey:     getEnv("PAYSTACK_PUBLIC_KEY", ""),
			PaystackWebhookSecret: getEnv("PAYSTACK_WEBHOOK_SECRET", ""),

			// Flutterwave
			FlutterwaveSecretKey:   getEnv("FLUTTERWAVE_SECRET_KEY", ""),
			FlutterwavePublicKey:   getEnv("FLUTTERWAVE_PUBLIC_KEY", ""),
			FlutterwaveWebhookHash: getEnv("FLUTTERWAVE_WEBHOOK_HASH", ""),

			// ImageKit
			ImageKitPublicKey:   getEnv("IMAGEKIT_PUBLIC_KEY", ""),
			ImageKitPrivateKey:  getEnv("IMAGEKIT_PRIVATE_KEY", ""),
			ImageKitURLEndpoint: getEnv("IMAGEKIT_URL_ENDPOINT", ""),

			// Platform
			PlatformFeePercent: getEnvInt("PLATFORM_FEE_PERCENT", 5),
		}

		// ── Required fields validation ───────────────────────────────────────
		if cfg.DBURL == "" {
			log.Fatal("DATABASE_URL is required.\n" +
				"→ Go to your Leapcell dashboard\n" +
				"→ Copy the full PostgreSQL connection string (starts with postgresql://)\n" +
				"→ Paste it into .env as: DATABASE_URL=postgresql://...")
		}

		if cfg.JWTSecret == "" {
			log.Fatal("JWT_SECRET is required.\n" +
				"Generate a strong random value (≥32 characters)\n" +
				"Example: openssl rand -base64 48")
		}

		if len(cfg.JWTSecret) < 32 {
			log.Println("WARNING: JWT_SECRET is weak (< 32 characters) — consider a longer random value!")
		}

		// ── Soft warnings for external services ──────────────────────────────
		// These are not fatal so the app can still start in development without
		// all payment and email keys configured.
		if cfg.ResendAPIKey == "" {
			log.Println("WARNING: RESEND_API_KEY is not set — emails will not be sent")
		}
		if cfg.PaystackSecretKey == "" {
			log.Println("WARNING: PAYSTACK_SECRET_KEY is not set — Paystack payments will not work")
		}
		if cfg.FlutterwaveSecretKey == "" {
			log.Println("WARNING: FLUTTERWAVE_SECRET_KEY is not set — Flutterwave payments will not work")
		}
		if cfg.ImageKitPrivateKey == "" {
			log.Println("WARNING: IMAGEKIT_PRIVATE_KEY is not set — file uploads will not work")
		}
		if cfg.PaystackWebhookSecret == "" {
			log.Println("WARNING: PAYSTACK_WEBHOOK_SECRET is not set — webhook verification will fail")
		}

		instance = cfg
	})

	return instance
}

// PostgresDSN returns the connection string for GORM / pgx.
func (c *Config) PostgresDSN() string {
	return c.DBURL
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// ── Helper functions ─────────────────────────────────────────────────────────

func splitTrim(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	// Support "7d", "30d" notation
	if strings.HasSuffix(v, "d") {
		var days float64
		_, err := fmt.Sscanf(strings.TrimSuffix(v, "d"), "%f", &days)
		if err == nil {
			return time.Duration(days * 24 * float64(time.Hour))
		}
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("Invalid duration for %s: %q → using fallback", key, v)
		return fallback
	}
	return d
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		log.Printf("Invalid integer for %s: %q → fallback used", key, v)
		return fallback
	}
	return n
}

func Get() *Config {
	return Load()
}
