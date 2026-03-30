// internal/api/middleware/rbac.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ── Context keys ──────────────────────────────────────────────────────────────
const (
	ContextKeyUserID      = "user_id"
	ContextKeyBusinessID  = "business_id"
	ContextKeyRole        = "role"
	ContextKeyIsPlatform  = "is_platform"
	ContextKeyAdminID     = "admin_id"
	ContextKeyMFAVerified = "mfa_verified"
)

// ── Business user roles ───────────────────────────────────────────────────────
const (
	RoleOwner = "owner"
	RoleStaff = "staff"
)

// ── Platform admin roles ──────────────────────────────────────────────────────
// Stored as platform_admins.role ("super_admin", "support", "finance").
// The JWT carries "platform_<role>" so these values include the prefix.
const (
	AdminRoleSuperAdmin = "platform_super_admin" // full access
	AdminRoleSupport    = "platform_support"     // read-only
	AdminRoleFinance    = "platform_finance"     // revenue/payouts only
)

// ── User RBAC ─────────────────────────────────────────────────────────────────

// RequireRole enforces business-user roles after AuthMiddleware.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, exists := c.Get(ContextKeyRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false, "message": "authentication required",
			})
			return
		}
		roleStr, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false, "message": "invalid role claim",
			})
			return
		}
		if _, ok := allowed[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false, "message": "you do not have permission to perform this action",
				"your_role": roleStr,
			})
			return
		}
		c.Next()
	}
}

// ── Admin RBAC ────────────────────────────────────────────────────────────────

// RequireAdminRole enforces platform admin roles after AdminAuthMiddleware.
// Use the AdminRole* constants as arguments.
//
//	admins.POST("/invite",  middleware.RequireAdminRole(AdminRoleSuperAdmin), ...)
//	analytics.GET("/revenue", middleware.RequireAdminRole(AdminRoleSuperAdmin, AdminRoleFinance), ...)
func RequireAdminRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		isPlatform, exists := c.Get(ContextKeyIsPlatform)
		if !exists || isPlatform != true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false, "message": "platform admin access required",
			})
			return
		}
		role, exists := c.Get(ContextKeyRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false, "message": "authentication required",
			})
			return
		}
		roleStr, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false, "message": "invalid role claim",
			})
			return
		}
		if _, ok := allowed[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success":   false,
				"message":   "your admin role does not have permission for this action",
				"your_role": roleStr,
			})
			return
		}
		c.Next()
	}
}

// RequirePlatformAdmin allows any authenticated admin regardless of sub-role.
// Use for routes every admin can reach (e.g. their own profile, dashboard).
func RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, exists := c.Get(ContextKeyIsPlatform)
		if !exists || isPlatform != true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false, "message": "platform admin access required",
			})
			return
		}
		c.Next()
	}
}
