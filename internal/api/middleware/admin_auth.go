// internal/api/middleware/admin_auth.go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/pkg/jwtpkg"
)

// AdminAuthMiddleware validates platform admin JWT tokens.
// It uses the ADMIN-specific JWT secret (separate from the user secret).
// It rejects any token that is not issued by "ogaos-admin" or has MFAVerified=false.
func AdminAuthMiddleware(adminSecret []byte) gin.HandlerFunc {
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
		claims, err := jwtpkg.ParseAdminToken(token, adminSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid or expired token",
			})
			return
		}

		// ParseAdminToken already checks IsPlatform and issuer.
		// We check MFAVerified here as an extra explicit gate.
		if !claims.MFAVerified {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "MFA verification required — complete 2FA to continue",
			})
			return
		}

		c.Set("admin_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("is_platform", true)
		c.Set("mfa_verified", true)

		c.Next()
	}
}
