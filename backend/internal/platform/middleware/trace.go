package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const TraceIDHeader = "X-Trace-ID"

type traceIDKey struct{}

// TraceID generates or propagates a trace_id per request. If the incoming
// request carries X-Trace-ID it is reused; otherwise a 32-hex-char random
// ID is created. The trace_id is injected into the context (accessible via
// TraceIDFromContext) and echoed in the response header.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader(TraceIDHeader)
		if tid == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			tid = hex.EncodeToString(b)
		}
		c.Header(TraceIDHeader, tid)
		ctx := context.WithValue(c.Request.Context(), traceIDKey{}, tid)
		c.Request = c.Request.WithContext(ctx)
		c.Set("trace_id", tid)
		c.Next()
	}
}

// TraceIDFromContext returns the trace id from a context, or "" if not set.
func TraceIDFromContext(ctx context.Context) string {
	if v := ctx.Value(traceIDKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
