package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets baseline security response headers on every response.
//
// The API serves JSON only (no HTML rendered server-side), so the CSP is
// maximally restrictive and framing is denied outright — there is no legitimate
// reason to embed API responses in a document. HSTS is emitted for TLS
// deployments; browsers ignore it over plain HTTP, so it is safe to always send
// (a TLS terminator/CDN normally sits in front in production).
//
// Added after a security review (ember sec-headers): the middleware stack
// previously set CORS but no HSTS/CSP/nosniff/frame-options.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		c.Next()
	}
}
