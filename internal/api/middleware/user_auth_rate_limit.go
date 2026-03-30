package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	goredis "github.com/redis/go-redis/v9"
)

// UserRegisterRateLimiter
// 3 requests/minute using IP + email when possible.
func UserRegisterRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:user:register",
		Limit:     3,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			ip := normalizeValue(c.ClientIP())

			var body struct {
				Email string `json:"email"`
			}
			_ = c.ShouldBindBodyWith(&body, binding.JSON)

			email := normalizeEmail(body.Email)
			if email == "" {
				return ip
			}
			return ip + ":" + email
		},
	})
}

// UserLoginRateLimiter
// 5 requests/minute using IP + email when possible.
func UserLoginRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:user:login",
		Limit:     5,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			ip := normalizeValue(c.ClientIP())

			var body struct {
				Email string `json:"email"`
			}
			_ = c.ShouldBindBodyWith(&body, binding.JSON)

			email := normalizeEmail(body.Email)
			if email == "" {
				return ip
			}
			return ip + ":" + email
		},
	})
}

// UserVerifyEmailRateLimiter
// 10 requests/minute using IP + token when possible.
func UserVerifyEmailRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:user:verify-email",
		Limit:     10,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			ip := normalizeValue(c.ClientIP())

			var body struct {
				Token string `json:"token"`
			}
			_ = c.ShouldBindBodyWith(&body, binding.JSON)

			token := strings.TrimSpace(body.Token)
			if token == "" {
				return ip
			}
			return ip + ":" + token
		},
	})
}

// UserResendVerificationRateLimiter
// 3 requests/minute using IP + email when possible.
func UserResendVerificationRateLimiter(redisClient *goredis.Client) gin.HandlerFunc {
	return NewRateLimitMiddleware(redisClient, RateLimitConfig{
		KeyPrefix: "rl:user:resend-verification",
		Limit:     3,
		Window:    time.Minute,
		KeyBuilder: func(c *gin.Context) string {
			ip := normalizeValue(c.ClientIP())

			var body struct {
				Email string `json:"email"`
			}
			_ = c.ShouldBindBodyWith(&body, binding.JSON)

			email := normalizeEmail(body.Email)
			if email == "" {
				return ip
			}
			return ip + ":" + email
		},
	})
}
