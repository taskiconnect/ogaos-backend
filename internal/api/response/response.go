// internal/api/response/response.go
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ─── Response shapes ──────────────────────────────────────────────────────────
//
// Every endpoint in the API returns one of these four shapes:
//
//  Success (no data):   { "success": true,  "message": "..." }
//  Success (with data): { "success": true,  "data": {...}    }
//  Paginated list:      { "success": true,  "data": [...], "meta": { "total", "page", "limit" } }
//  Error:               { "success": false, "message": "..." }

// OK sends 200 with a data payload.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// Created sends 201 with a data payload.
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

// Message sends 200 with a plain success message and no data.
// Use for mutations that don't need to return the updated resource
// (e.g. delete, cancel, set-default).
func Message(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
}

// List sends 200 with a paginated data array and meta block.
func List(c *gin.Context, data interface{}, total int64, page, limit int) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"meta": gin.H{
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// BadRequest sends 400.
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": msg})
}

// Unauthorized sends 401.
func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": msg})
}

// Forbidden sends 403.
func Forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "message": msg})
}

// NotFound sends 404.
func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"success": false, "message": msg})
}

// InternalError sends 500.
func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": msg})
}

// Err is a convenience helper that picks the right status code based on common
// service-layer error strings, so handlers don't need to do that themselves.
// Extend the switch as new error patterns emerge.
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
