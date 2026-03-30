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

	// Database
	DBURL string

	// JWT
	JWTSecret        string
	AdminJWTSecret   string
	JWTExpiry        time.Duration
	AdminJWTExpiry   time.Duration
	JWTRefreshExpiry time.Duration
	AdminRefreshTTL  time.Duration

	// Security / API
	AllowedOrigins []string
	TrustedProxies []string

	// Frontend
	FrontendURL string

	// Redis / Upstash
	UpstashRedisURL string

	// Email
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
	PlatformFeePercent int
}

var (
	instance *Config
	once     sync.Once
)

func Load() *Config {
	once.Do(func() {
		_ = godotenv.Load()

		cfg := &Config{
			Port:    getEnv("PORT", "8080"),
			Env:     getEnv("ENV", "development"),
			AppName: getEnv("APP_NAME", "OgaOs"),

			DBURL: getEnv("DATABASE_URL", ""),

			JWTSecret:        getEnv("JWT_SECRET", ""),
			AdminJWTSecret:   getEnv("ADMIN_JWT_SECRET", ""),
			JWTExpiry:        getEnvDuration("JWT_EXPIRY", 15*time.Minute),
			AdminJWTExpiry:   getEnvDuration("ADMIN_JWT_EXPIRY", 15*time.Minute),
			JWTRefreshExpiry: getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
			AdminRefreshTTL:  getEnvDuration("ADMIN_REFRESH_TTL", 24*time.Hour),

			AllowedOrigins: splitTrim(
				getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"),
			),
			TrustedProxies: splitTrim(getEnv("TRUSTED_PROXIES", "")),

			FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),

			UpstashRedisURL: getEnv("UPSTASH_REDIS_URL", ""),

			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			EmailFrom:    getEnv("EMAIL_FROM", "hello@ogaos.taskiconnet.com"),

			PaystackSecretKey:     getEnv("PAYSTACK_SECRET_KEY", ""),
			PaystackPublicKey:     getEnv("PAYSTACK_PUBLIC_KEY", ""),
			PaystackWebhookSecret: getEnv("PAYSTACK_WEBHOOK_SECRET", ""),

			FlutterwaveSecretKey:   getEnv("FLUTTERWAVE_SECRET_KEY", ""),
			FlutterwavePublicKey:   getEnv("FLUTTERWAVE_PUBLIC_KEY", ""),
			FlutterwaveWebhookHash: getEnv("FLUTTERWAVE_WEBHOOK_HASH", ""),

			ImageKitPublicKey:   getEnv("IMAGEKIT_PUBLIC_KEY", ""),
			ImageKitPrivateKey:  getEnv("IMAGEKIT_PRIVATE_KEY", ""),
			ImageKitURLEndpoint: getEnv("IMAGEKIT_URL_ENDPOINT", ""),

			PlatformFeePercent: getEnvInt("PLATFORM_FEE_PERCENT", 5),
		}

		validate(cfg)
		instance = cfg
	})

	return instance
}

func Get() *Config {
	return Load()
}

// Backward-compatible helper for existing DB code.
func (c *Config) PostgresDSN() string {
	return c.DBURL
}

func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func validate(cfg *Config) {
	if cfg.DBURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}
	if cfg.AdminJWTSecret == "" {
		log.Fatal("ADMIN_JWT_SECRET is required")
	}
	if len(cfg.AdminJWTSecret) < 32 {
		log.Fatal("ADMIN_JWT_SECRET must be at least 32 characters")
	}
	if cfg.AdminJWTSecret == cfg.JWTSecret {
		log.Fatal("ADMIN_JWT_SECRET must differ from JWT_SECRET")
	}
	if cfg.UpstashRedisURL == "" {
		log.Fatal("UPSTASH_REDIS_URL is required")
	}

	if cfg.ResendAPIKey == "" {
		log.Println("WARNING: RESEND_API_KEY not set — emails may fail")
	}
	if cfg.PaystackSecretKey == "" {
		log.Println("WARNING: PAYSTACK_SECRET_KEY not set — Paystack may fail")
	}
	if cfg.ImageKitPrivateKey == "" {
		log.Println("WARNING: IMAGEKIT_PRIVATE_KEY not set — uploads may fail")
	}
}

func splitTrim(s string) []string {
	if s == "" {
		return nil
	}

	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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

	if strings.HasSuffix(v, "d") {
		var days float64
		if _, err := fmt.Sscanf(strings.TrimSuffix(v, "d"), "%f", &days); err == nil {
			return time.Duration(days * 24 * float64(time.Hour))
		}
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid duration for %s: %q — using fallback %s", key, v, fallback)
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
		log.Printf("invalid int for %s: %q — using fallback %d", key, v, fallback)
		return fallback
	}

	return n
}
