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
		// Bearer-token auth (no cookies) → return the configured origin directly.
		// We never reflect the request's Origin, so an attacker page can't get
		// itself allowlisted. "*" allows any (dev only); set a specific origin
		// in production.
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
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
