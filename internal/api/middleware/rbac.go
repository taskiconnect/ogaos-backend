// internal/api/middleware/rbac.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Context keys set by AuthMiddleware — read by all handlers and middleware
const (
	ContextKeyUserID     = "user_id"
	ContextKeyBusinessID = "business_id"
	ContextKeyRole       = "role"
	ContextKeyIsPlatform = "is_platform"
)

const (
	RoleOwner = "owner"
	RoleStaff = "staff"
)

// RequireRole returns middleware that only allows the specified roles.
// Must be used after AuthMiddleware.
//
//	r.POST("/products", middleware.RequireRole(RoleOwner), productHandler.Create)
//	r.GET("/ledger",    middleware.RequireRole(RoleOwner, RoleStaff), ledgerHandler.List)
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, exists := c.Get(ContextKeyRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "authentication required",
			})
			return
		}
		roleStr, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "invalid role claim",
			})
			return
		}
		if _, ok := allowed[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success":   false,
				"message":   "you do not have permission to perform this action",
				"your_role": roleStr,
			})
			return
		}
		c.Next()
	}
}

// RequirePlatformAdmin allows only platform admins through.
func RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, exists := c.Get(ContextKeyIsPlatform)
		if !exists || isPlatform != true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "platform admin access required",
			})
			return
		}
		c.Next()
	}
}
