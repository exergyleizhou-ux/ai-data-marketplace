package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS allows browser clients (the Next.js frontend) to call the API
// cross-origin. Auth uses bearer tokens (not cookies), so we can reflect the
// configured origin without credentials. allowedOrigin "*" allows any (dev);
// set a specific origin in production.
func CORS(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := allowedOrigin
		if allowedOrigin != "*" {
			// Reflect only when the request origin matches the allowlist.
			if reqOrigin := c.GetHeader("Origin"); reqOrigin != allowedOrigin {
				origin = allowedOrigin
			}
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Signature, Idempotency-Key")
		c.Header("Access-Control-Max-Age", "600")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
