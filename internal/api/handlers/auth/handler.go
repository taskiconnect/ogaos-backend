// internal/api/handlers/auth/handler.go
package auth

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	apperr "ogaos-backend/internal/pkg/errors"
	svc "ogaos-backend/internal/service/auth"
)

type Handler struct {
	service *svc.AuthService
	secure  bool // true in production — controls Secure flag on cookies
	log     *slog.Logger
}

func NewHandler(service *svc.AuthService, isProduction bool, log *slog.Logger) *Handler {
	return &Handler{service: service, secure: isProduction, log: log}
}

// Register — create a new business owner account
func (h *Handler) Register(c *gin.Context) {
	var req svc.RegisterRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if err := h.service.Register(req); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "account created — check your email to verify before logging in",
	})
}

// VerifyEmail — POST /auth/verify  (token in JSON body)
func (h *Handler) VerifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "verification token is required"})
		return
	}
	if err := h.service.VerifyEmail(req.Token); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "email verified — you can now log in"})
}

// ResendVerification — always returns 200 (no email enumeration)
func (h *Handler) ResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	_ = h.service.ResendVerification(req.Email) // errors are swallowed intentionally
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "if that email is registered and unverified, a new link has been sent",
	})
}

// Login — email + password → access token (cookie: refresh token)
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email"    binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	accessToken, refreshToken, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	h.setRefreshCookie(c, refreshToken)
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": accessToken,
		"message":      "login successful",
	})
}

// Refresh — rotate refresh token → new access + refresh
func (h *Handler) Refresh(c *gin.Context) {
	rawToken, err := c.Cookie("refresh_token")
	if err != nil || rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "no refresh token provided"})
		return
	}
	newAccess, newRefresh, err := h.service.Refresh(rawToken)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	h.setRefreshCookie(c, newRefresh)
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": newAccess,
		"message":      "tokens refreshed successfully",
	})
}

// Logout — revoke refresh token, clear cookie
func (h *Handler) Logout(c *gin.Context) {
	rawToken, err := c.Cookie("refresh_token")
	if err == nil && rawToken != "" {
		_ = h.service.Logout(rawToken)
	}
	c.SetCookie("refresh_token", "", -1, "/", "", h.secure, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "logged out successfully"})
}

// WhoAmI — return the authenticated user's profile
func (h *Handler) WhoAmI(c *gin.Context) {
	userIDVal, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "something went wrong, please try again"})
		return
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "something went wrong, please try again"})
		return
	}
	isPlatform := c.GetBool("is_platform")
	resp, err := h.service.WhoAmI(userID, isPlatform)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// ── Staff management (owner only — RBAC enforced by middleware) ───────────────

func (h *Handler) CreateStaff(c *gin.Context) {
	businessID, ok := mustBusinessID(c)
	if !ok {
		return
	}
	var req svc.StaffCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if err := h.service.CreateStaff(businessID, req); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "staff member created — they will receive a verification email",
	})
}

func (h *Handler) DeactivateStaff(c *gin.Context) {
	businessID, ok := mustBusinessID(c)
	if !ok {
		return
	}
	staffIDStr := c.Param("id")
	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid staff id"})
		return
	}
	if err := h.service.DeactivateStaff(businessID, staffID); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "staff member deactivated"})
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (h *Handler) setRefreshCookie(c *gin.Context, token string) {
	c.SetCookie(
		"refresh_token",
		token,
		3600*24*7, // 7 days
		"/",
		"",
		h.secure,
		true, // HttpOnly
	)
}

// mustBusinessID extracts business_id from gin context, writing a 500 if missing.
func mustBusinessID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("business_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "something went wrong, please try again"})
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "something went wrong, please try again"})
		return uuid.Nil, false
	}
	return id, true
}
