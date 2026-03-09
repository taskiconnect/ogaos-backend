package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/pkg/jwtpkg"
)

func AuthMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "missing or invalid authorization header"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := jwtpkg.ParseAccessToken(token, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid token"})
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

// Optional: restrict to platform admins only
func PlatformAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform := c.GetBool("is_platform")
		if !isPlatform {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "platform admin access required"})
			return
		}
		c.Next()
	}
}
