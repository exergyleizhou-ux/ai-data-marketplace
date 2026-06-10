package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds production-hardened HTTP response headers.  Run this
// AFTER CORS middleware so it doesn't step on Access-Control-* headers.
//
// HSTS is only set over HTTPS (enforce in your reverse-proxy / LB).
// CSP is report-only in development (Content-Security-Policy-Report-Only) so
// the team can tune it before enforcing.
func SecurityHeaders(env string) gin.HandlerFunc {
	hsts := "max-age=63072000; includeSubDomains; preload"
	if env == "development" {
		hsts = "max-age=0"
	}

	return func(c *gin.Context) {
		// Prevent MIME-type sniffing (IE/old Edge).
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking.  If you embed in an iframe intentionally,
		// override this on specific routes.
		c.Header("X-Frame-Options", "DENY")

		// Limit referrer leakage: send full referrer only for same-origin.
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Enable browser XSS auditor.
		c.Header("X-XSS-Protection", "0")

		// Strict Transport Security (only meaningful over HTTPS).
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Header("Strict-Transport-Security", hsts)
		}

		// Permissions-Policy: restrict browser features.
		c.Header("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), "+
				"payment=(), usb=(), magnetometer=(), gyroscope=()")

		// Content-Security-Policy: allow self + specific CDN origins.
		// img-src allows data: so TOTP QR codes render (they're data: URIs).
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; " +
			"connect-src 'self'; " +
			"font-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		if env == "development" {
			c.Header("Content-Security-Policy-Report-Only", csp)
		} else {
			c.Header("Content-Security-Policy", csp)
		}

		c.Next()
	}
}

// CacheControl adds Cache-Control: no-store to authenticated responses and
// all API routes.  This prevents browsers/CDNs from caching sensitive data.
func CacheControl() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Next()
	}
}

// RemoveServerHeader strips the Gin/Go version header that leaks server
// implementation details.
func RemoveServerHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Server", "")
		c.Next()
	}
}

// OptionsTerminator short-circuits OPTIONS preflight requests at the
// middleware layer, before they reach handlers.  The CORS middleware
// already handles the actual CORS headers; this just stops Gin from
// logging 404 for preflight requests to unregistered OPTIONS handlers.
func OptionsTerminator() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
