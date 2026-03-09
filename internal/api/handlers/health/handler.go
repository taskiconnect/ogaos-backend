// internal/api/handlers/health/handler.go
package health

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the HTTP handler layer for health check endpoints
type Handler struct {
	db *gorm.DB
}

// NewHandler creates a new health handler instance
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// Check returns the health status of the service and its dependencies
func (h *Handler) Check(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":   false,
			"status":    "degraded",
			"database":  "unreachable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"status":    "ok",
		"database":  "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
