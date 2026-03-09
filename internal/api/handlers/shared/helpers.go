// internal/api/handlers/shared/helpers.go
package shared

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ogaos-backend/internal/api/middleware"
	"ogaos-backend/internal/api/response"
)

// MustBusinessID extracts the business UUID set by AuthMiddleware.
func MustBusinessID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(middleware.ContextKeyBusinessID)
	return v.(uuid.UUID)
}

// MustUserID extracts the user UUID set by AuthMiddleware.
func MustUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(middleware.ContextKeyUserID)
	return v.(uuid.UUID)
}

// ParseID parses a URL param as uuid.UUID.
// Writes a 400 response and returns false on failure — the handler should return immediately.
func ParseID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		response.BadRequest(c, "invalid "+param)
		return uuid.Nil, false
	}
	return id, true
}

// Paginate returns page and limit from query params with sane defaults.
func Paginate(c *gin.Context) (page, limit int) {
	page = queryInt(c, "page", 1)
	limit = queryInt(c, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	return
}

// QueryUUID returns a query param as *uuid.UUID, or nil if missing/invalid.
func QueryUUID(c *gin.Context, key string) *uuid.UUID {
	s := c.Query(key)
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// QueryTime parses a query param as RFC3339 time, or returns nil if missing/invalid.
func QueryTime(c *gin.Context, key string) *time.Time {
	s := c.Query(key)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// QueryBool returns true when a query param equals "true".
func QueryBool(c *gin.Context, key string) bool {
	return c.Query(key) == "true"
}

// QueryInt returns a query param as int, or fallback if missing/invalid.
func QueryInt(c *gin.Context, key string, fallback int) int {
	return queryInt(c, key, fallback)
}

// ─── unexported ───────────────────────────────────────────────────────────────

func queryInt(c *gin.Context, key string, fallback int) int {
	s := c.Query(key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
