package admin_auth

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperr "ogaos-backend/internal/pkg/errors"
	svc "ogaos-backend/internal/service/admin_auth"
)

type Handler struct {
	service *svc.AdminAuthService
	secure  bool
	log     *slog.Logger
}

func NewHandler(service *svc.AdminAuthService, isProduction bool, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		secure:  isProduction,
		log:     log,
	}
}

// Login
func (h *Handler) Login(c *gin.Context) {
	var req svc.AdminLoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("admin login bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	resp, err := h.service.Login(req)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// VerifyOTP
func (h *Handler) VerifyOTP(c *gin.Context) {
	var req svc.AdminOTPVerificationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("admin verify otp bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	resp, refreshToken, err := h.service.VerifyOTP(req)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	h.setRefreshCookie(c, refreshToken)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": resp.AccessToken,
		"message":      resp.Message,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	rawToken, err := c.Cookie("admin_refresh_token")
	if err != nil || rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "no refresh token provided",
		})
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

func (h *Handler) Logout(c *gin.Context) {
	rawToken, err := c.Cookie("admin_refresh_token")
	if err == nil && rawToken != "" {
		_ = h.service.Logout(rawToken)
	}

	c.SetCookie("admin_refresh_token", "", -1, "/api/v1/admin", "", h.secure, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "logged out successfully",
	})
}

func (h *Handler) ResendOTP(c *gin.Context) {
	var req struct {
		TempToken string `json:"temp_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("admin resend otp bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.ResendOTP(req.TempToken); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "new OTP sent to your email",
	})
}

func (h *Handler) SetupPassword(c *gin.Context) {
	var req svc.AdminPasswordSetupRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("admin setup password bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.SetupPassword(req); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "password set successfully — you can now log in",
	})
}

// WhoAmI returns the authenticated admin's own profile.
func (h *Handler) WhoAmI(c *gin.Context) {
	adminID, ok := mustAdminID(c)
	if !ok {
		return
	}

	profile, err := h.service.WhoAmI(adminID)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    profile,
	})
}

// InviteAdmin creates a new platform admin and sends them a setup email.
func (h *Handler) InviteAdmin(c *gin.Context) {
	var req svc.InviteAdminRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("invite admin bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.InviteAdmin(req); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "admin invited — a setup email has been sent",
	})
}

func (h *Handler) ListAdmins(c *gin.Context) {
	admins, err := h.service.ListAdmins()
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    admins,
	})
}

func (h *Handler) GetAdmin(c *gin.Context) {
	targetID, ok := parseAdminID(c)
	if !ok {
		return
	}

	profile, err := h.service.GetAdmin(targetID)
	if err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    profile,
	})
}

func (h *Handler) UpdateAdmin(c *gin.Context) {
	callerID, ok := mustAdminID(c)
	if !ok {
		return
	}

	targetID, ok := parseAdminID(c)
	if !ok {
		return
	}

	var req svc.UpdateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("update admin bind failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.UpdateAdmin(callerID, targetID, req); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "admin updated successfully",
	})
}

func (h *Handler) DeactivateAdmin(c *gin.Context) {
	callerID, ok := mustAdminID(c)
	if !ok {
		return
	}

	targetID, ok := parseAdminID(c)
	if !ok {
		return
	}

	if err := h.service.DeactivateAdmin(callerID, targetID); err != nil {
		apperr.Respond(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "admin deactivated successfully",
	})
}

func (h *Handler) setRefreshCookie(c *gin.Context, token string) {
	c.SetCookie("admin_refresh_token", token, 3600*24, "/api/v1/admin", "", h.secure, true)
}

func mustAdminID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("admin_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "something went wrong, please try again",
		})
		return uuid.Nil, false
	}

	id, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "something went wrong, please try again",
		})
		return uuid.Nil, false
	}

	return id, true
}

func parseAdminID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid admin id",
		})
		return uuid.Nil, false
	}

	return id, true
}
