// internal/pkg/errors/apperr.go
//
// Centralised application-error vocabulary.
//
// Usage in services:
//
//	return apperr.ErrInvalidCredentials
//	return apperr.New(apperr.CodeNotFound, "admin not found")
//
// Usage in handlers — respond + log in one call:
//
//	apperr.Respond(c, log, err)
package apperr

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ── Error codes ───────────────────────────────────────────────────────────────

type Code int

const (
	CodeBadRequest   Code = iota // 400 — caller mistake, safe to surface
	CodeUnauthorized             // 401 — auth failure
	CodeForbidden                // 403 — authenticated but not allowed
	CodeNotFound                 // 404
	CodeConflict                 // 409 — e.g. duplicate resource
	CodeInternal                 // 500 — hide detail from client
)

// ── AppError ──────────────────────────────────────────────────────────────────

// AppError carries a safe public message and an HTTP status derived from its code.
type AppError struct {
	Code    Code
	Message string // safe to send to clients
	cause   error  // internal; never serialised
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.cause }

// New constructs an AppError. Pass a non-nil cause for server-side logging.
func New(code Code, msg string, cause ...error) *AppError {
	ae := &AppError{Code: code, Message: msg}
	if len(cause) > 0 {
		ae.cause = cause[0]
	}
	return ae
}

// Wrap wraps an internal error with a safe public message.
func Wrap(code Code, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, cause: cause}
}

// ── Pre-defined sentinel errors ───────────────────────────────────────────────

var (
	// Auth
	ErrInvalidCredentials = New(CodeUnauthorized, "invalid email or password")
	ErrInvalidToken       = New(CodeUnauthorized, "invalid or expired token")
	ErrAccountDeactivated = New(CodeForbidden, "your account has been deactivated")
	ErrForbidden          = New(CodeForbidden, "you do not have permission to perform this action")

	// Resources
	ErrNotFound = New(CodeNotFound, "resource not found")
	ErrConflict = New(CodeConflict, "resource already exists")

	// Generic
	ErrInternal = New(CodeInternal, "something went wrong, please try again")
)

// ── HTTP status mapping ───────────────────────────────────────────────────────

func httpStatus(code Code) int {
	switch code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// ── Handler helper ────────────────────────────────────────────────────────────

// Respond writes the correct JSON error response and logs the internal cause
// when present. Pass a *slog.Logger (or nil to skip logging).
//
//	apperr.Respond(c, logger, err)
func Respond(c *gin.Context, log *slog.Logger, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		// Log internal cause if available
		if log != nil && ae.cause != nil {
			log.Error("request error",
				"public_message", ae.Message,
				"internal_cause", ae.cause.Error(),
				"path", c.FullPath(),
				"method", c.Request.Method,
			)
		}
		c.JSON(httpStatus(ae.Code), gin.H{"success": false, "message": ae.Message})
		return
	}

	// Unknown / unwrapped error — log it and return generic message
	if log != nil {
		log.Error("unhandled error",
			"error", err.Error(),
			"path", c.FullPath(),
			"method", c.Request.Method,
		)
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": ErrInternal.Message,
	})
}
