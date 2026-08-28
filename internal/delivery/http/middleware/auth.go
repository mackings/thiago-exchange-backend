package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/auth"
)

const (
	ctxUserID = "userID"
	ctxRole   = "role"
)

// RequireAuth reads the access token from the Authorization header (or the
// "access_token" cookie as a fallback for browser navigations/WS upgrades
// that can't set custom headers) and stashes the caller's identity in the
// gin context for handlers to read via CurrentUserID/CurrentRole.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := auth.ParseToken(secret, token)
		if err != nil || claims.Type != auth.TokenAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxRole, claims.Role)
		c.Next()
	}
}

// RequireAdmin must run after RequireAuth.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(ctxRole)
		if role != domain.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin only"})
			return
		}
		c.Next()
	}
}

// extractToken checks, in order: the Authorization header (normal API
// calls), a "token" query param (WebSocket upgrades — browsers can't set
// custom headers on the handshake), then an access_token cookie fallback.
func extractToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if token := c.Query("token"); token != "" {
		return token
	}
	if cookie, err := c.Cookie("access_token"); err == nil {
		return cookie
	}
	return ""
}
