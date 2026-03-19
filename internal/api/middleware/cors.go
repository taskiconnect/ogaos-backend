// internal/api/middleware/cors.go
package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	// Always include the core allowed origins regardless of what is passed in.
	// This ensures local development and production both work without extra env config.
	coreOrigins := []string{
		"http://localhost:3000",
		"https://ogaos.taskiconnect.com",
	}

	// Merge: combine coreOrigins with whatever the caller passed in,
	// deduplicated so we don't list the same origin twice.
	seen := make(map[string]bool)
	merged := make([]string, 0, len(coreOrigins)+len(allowedOrigins))
	for _, o := range append(coreOrigins, allowedOrigins...) {
		if !seen[o] {
			seen[o] = true
			merged = append(merged, o)
		}
	}

	return cors.New(cors.Config{
		AllowOrigins:     merged,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true, // needed for cookies (refresh token)
		MaxAge:           12 * time.Hour,
	})
}
