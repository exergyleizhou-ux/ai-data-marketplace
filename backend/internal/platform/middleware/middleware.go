// Package middleware holds the cross-cutting Gin middleware stack: request-id
// propagation, structured access logging, and panic recovery that renders the
// uniform error envelope. Auth/RBAC and rate limiting are added in PR-04/06.
package middleware

import (
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// RequestID assigns a correlation id to every request (honouring an inbound
// X-Request-ID), stores it in the context, and echoes it in the response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(httpx.RequestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(httpx.RequestIDKey, rid)
		c.Writer.Header().Set(httpx.RequestIDHeader, rid)
		c.Next()
	}
}

// Logger emits one structured access-log line per request after it completes.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http_request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"request_id", httpx.RequestID(c),
			"trace_id", c.GetString("trace_id"),
		)
	}
}

// Recovery converts an unhandled panic into a logged, uniform 500 envelope
// instead of dropping the connection.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered",
					"err", r,
					"path", c.Request.URL.Path,
					"request_id", httpx.RequestID(c),
					"stack", string(debug.Stack()),
				)
				httpx.Fail(c, httpx.ErrInternal)
				c.Abort()
			}
		}()
		c.Next()
	}
}
