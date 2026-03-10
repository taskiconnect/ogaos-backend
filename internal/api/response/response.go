// internal/api/response/response.go
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Every endpoint returns one of these shapes:
//
//  Success (no data):     { "success": true,  "message": "..." }
//  Success (with data):   { "success": true,  "data": {...}    }
//  Cursor-paginated list: { "success": true,  "data": [...], "next_cursor": "..." | null }
//  Error:                 { "success": false, "message": "..." }
//
// Cursor pagination:
//   Client sends  ?cursor=<opaque>&limit=20
//   Server returns next_cursor — null when no more pages exist.
//   No total count (avoids COUNT(*) on every list request).

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

func Message(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
}

// CursorList sends a cursor-paginated response.
// nextCursor is "" when there are no more pages — serialised as JSON null.
func CursorList(c *gin.Context, data interface{}, nextCursor string) {
	var next interface{} = nextCursor
	if nextCursor == "" {
		next = nil
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"data":        data,
		"next_cursor": next,
	})
}

func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": msg})
}

func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": msg})
}

func Forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "message": msg})
}

func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"success": false, "message": msg})
}

func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": msg})
}

func Err(c *gin.Context, err error) {
	msg := err.Error()
	switch msg {
	case "record not found", "not found":
		NotFound(c, msg)
	case "unauthorized", "authentication required":
		Unauthorized(c, msg)
	case "forbidden", "access denied":
		Forbidden(c, msg)
	default:
		BadRequest(c, msg)
	}
}
