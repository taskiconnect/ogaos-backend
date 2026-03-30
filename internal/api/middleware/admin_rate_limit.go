package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	pkgredis "ogaos-backend/internal/pkg/redis"
)

type RateLimitKeyBuilder func(c *gin.Context) string

type RateLimitConfig struct {
	KeyPrefix  string
	Limit      int
	Window     time.Duration
	KeyBuilder RateLimitKeyBuilder
}

func NewRateLimitMiddleware(redisClient *goredis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	limiter := pkgredis.NewRateLimiter(redisClient)

	return func(c *gin.Context) {
		keySuffix := "anonymous"

		if cfg.KeyBuilder != nil {
			if v := strings.TrimSpace(cfg.KeyBuilder(c)); v != "" {
				keySuffix = v
			}
		}

		key := cfg.KeyPrefix + ":" + keySuffix

		result, err := limiter.Allow(c.Request.Context(), key, cfg.Limit, cfg.Window)
		if err != nil {
			// Fail open so Redis problems do not break auth completely.
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", strconv.Itoa(result.ResetAfter))

		if !result.Allowed {
			if result.RetryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(result.RetryAfter))
			}

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "too many requests, please try again shortly",
			})
			return
		}

		c.Next()
	}
}

// AdminLoginRateLimiter
// Use IP only to avoid consuming/parsing the request body in middleware.
func AdminLoginRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:admin:login",
		Limit:     5,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			return normalizeValue(c.ClientIP())
		},
	})
}

// AdminVerifyOTPRateLimiter
// Use IP only for the same reason: do not read body in middleware.
func AdminVerifyOTPRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:admin:verify-otp",
		Limit:     5,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			return normalizeValue(c.ClientIP())
		},
	})
}

// AdminResendOTPRateLimiter
// Use IP only.
func AdminResendOTPRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:admin:resend-otp",
		Limit:     3,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			return normalizeValue(c.ClientIP())
		},
	})
}

func normalizeEmail(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizeValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}
