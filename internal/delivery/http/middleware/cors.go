package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS allows any of allowedOrigins (case-insensitive) — there's more than
// one legitimate frontend origin live at once during this migration (the
// custom domain, Netlify's default subdomain, and a Render-hosted
// frontend), so this can't be a single hardcoded value. Access-Control-
// Allow-Origin can only ever echo back one origin at a time (never "*"
// alongside credentials), so the request's own Origin is reflected back
// once it's confirmed to be on the allowlist.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(o)] = true
	}
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && allowed[strings.ToLower(origin)] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
