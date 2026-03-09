// internal/api/handlers/auth/handler.go
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ogaos-backend/internal/service/auth" // ← your service package
)

// Handler is the HTTP handler layer for authentication endpoints
type Handler struct {
	service *auth.AuthService
}

// NewHandler creates a new auth handler instance
func NewHandler(service *auth.AuthService) *Handler {
	return &Handler{
		service: service,
	}
}

// Register creates a new business owner account
func (h *Handler) Register(c *gin.Context) {
	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.Register(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Account created successfully. Please check your email to verify.",
	})
}

// VerifyEmail verifies the email using the token from the link
func (h *Handler) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Verification token is required",
		})
		return
	}

	if err := h.service.VerifyEmail(token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Email verified successfully. You can now log in.",
	})
}

// Login authenticates a user (owner, staff or platform admin)
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	accessToken, refreshToken, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Set HttpOnly + Secure cookie for refresh token
	c.SetCookie(
		"refresh_token",
		refreshToken,
		3600*24*7, // 7 days
		"/",
		"",
		true, // Secure (change to true in production with HTTPS)
		true, // HttpOnly
	)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": accessToken,
		"message":      "Login successful",
	})
}

// Refresh exchanges a valid refresh token for a new access + refresh pair
func (h *Handler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No refresh token provided",
		})
		return
	}

	newAccess, newRefresh, err := h.service.Refresh(refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.SetCookie(
		"refresh_token",
		newRefresh,
		3600*24*7,
		"/",
		"",
		true,
		true,
	)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": newAccess,
		"message":      "Tokens refreshed successfully",
	})
}

// Logout invalidates the current refresh token
func (h *Handler) Logout(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err == nil && refreshToken != "" {
		_ = h.service.Logout(refreshToken)
	}

	// Clear the cookie
	c.SetCookie("refresh_token", "", -1, "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

// ────────────────────────────────────────────────
// WhoAmI – returns the authenticated user's profile
// ────────────────────────────────────────────────

func (h *Handler) WhoAmI(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "user context missing",
		})
		return
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "invalid user ID type",
		})
		return
	}

	isPlatform := c.GetBool("is_platform")

	resp, err := h.service.WhoAmI(userID, isPlatform)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// ────────────────────────────────────────────────
// Staff Management (Owner only)
// ────────────────────────────────────────────────

func (h *Handler) CreateStaff(c *gin.Context) {
	businessIDVal, exists := c.Get("business_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "business context missing",
		})
		return
	}

	businessID, ok := businessIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "invalid business ID type",
		})
		return
	}

	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "role context missing",
		})
		return
	}

	role, ok := roleVal.(string)
	if !ok || role != "owner" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only the business owner can add staff",
		})
		return
	}

	var req auth.StaffCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := h.service.CreateStaff(businessID, req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Staff member created. They will receive an email to verify their account.",
	})
}

func (h *Handler) DeactivateStaff(c *gin.Context) {
	businessIDVal, exists := c.Get("business_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "business context missing",
		})
		return
	}

	businessID, ok := businessIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "invalid business ID type",
		})
		return
	}

	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "role context missing",
		})
		return
	}

	role, ok := roleVal.(string)
	if !ok || role != "owner" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only the business owner can deactivate staff",
		})
		return
	}

	staffIDStr := c.Param("id")
	if staffIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "staff id is required in the URL",
		})
		return
	}

	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid staff id format",
		})
		return
	}

	if err := h.service.DeactivateStaff(businessID, staffID); err != nil {
		code := http.StatusBadRequest
		if err.Error() == "staff member not found in this business" {
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Staff member deactivated successfully",
	})
}
