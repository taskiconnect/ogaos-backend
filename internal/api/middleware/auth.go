// internal/api/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/pkg/jwtpkg"
)

// AuthMiddleware validates regular user (owner/staff) access tokens.
// It uses the USER-specific JWT secret only.
// Platform admin tokens signed with the admin secret will fail here — by design.
func AuthMiddleware(userSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "missing or invalid authorization header",
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := jwtpkg.ParseAccessToken(token, userSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid or expired token",
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("business_id", claims.BusinessID)
		c.Set("role", claims.Role)
		c.Set("is_platform", claims.IsPlatform)

		c.Next()
	}
}
