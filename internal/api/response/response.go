// internal/api/response/response.go
package response

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

func Message(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
}

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
	if err == nil {
		InternalError(c, "something went wrong")
		return
	}

	log.Printf("[API_ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)

	msg := strings.TrimSpace(err.Error())
	lowerMsg := strings.ToLower(msg)

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		NotFound(c, "record not found")

	case msg == "record not found" || msg == "not found":
		NotFound(c, msg)

	case msg == "unauthorized" || msg == "authentication required":
		Unauthorized(c, msg)

	case msg == "forbidden" || msg == "access denied":
		Forbidden(c, msg)

	case isDatabaseError(lowerMsg):
		InternalError(c, "something went wrong, please try again later")

	default:
		BadRequest(c, msg)
	}
}

func isDatabaseError(msg string) bool {
	dbErrorMarkers := []string{
		"sqlstate",
		"relation",
		"duplicate key",
		"violates foreign key constraint",
		"violates unique constraint",
		"violates not-null constraint",
		"pq:",
		"syntax error at or near",
		"database",
		"deadlock",
		"connection refused",
		"connection reset",
		"no rows in result set",
	}

	for _, marker := range dbErrorMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}

	return false
}
