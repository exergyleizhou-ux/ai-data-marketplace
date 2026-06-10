package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// BodyLimitRule raises (or lowers) the cap for one gin route template,
// matched against c.FullPath() — e.g. the chunked dataset upload endpoint.
type BodyLimitRule struct {
	Route string // gin route template, e.g. "/api/v1/datasets/:id/upload/part"
	Max   int64
}

// BodyLimit caps request body size. Defense in depth:
//
//  1. Declared Content-Length over the cap → immediate 413 envelope, body
//     never read.
//  2. http.MaxBytesReader wraps the body as a backstop for chunked or lying
//     clients — a handler read stops at the cap with an error (the connection
//     is also closed), so memory/disk can't be exhausted even when the
//     request sneaks past the fast check. Those surface as read errors in
//     handlers (400/500 envelopes), which is acceptable for a hostile path.
func BodyLimit(defaultMax int64, rules ...BodyLimitRule) gin.HandlerFunc {
	return func(c *gin.Context) {
		max := defaultMax
		for _, r := range rules {
			if c.FullPath() == r.Route {
				max = r.Max
				break
			}
		}
		if c.Request.ContentLength > max {
			httpx.Fail(c, httpx.ErrBodyTooLarge)
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
