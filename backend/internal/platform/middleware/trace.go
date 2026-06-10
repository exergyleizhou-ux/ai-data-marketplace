package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const TraceIDHeader = "X-Trace-ID"

type traceIDKey struct{}

// TraceID generates or propagates a trace_id per request. Priority:
//  1. an active OTel span on the request context (set by otelgin when tracing
//     is enabled) — its trace ID is reused so logs correlate with traces;
//  2. an incoming X-Trace-ID header;
//  3. a fresh 32-hex-char random ID.
//
// The trace_id is injected into the context (accessible via
// TraceIDFromContext) and echoed in the response header.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tid string
		if sc := oteltrace.SpanContextFromContext(c.Request.Context()); sc.IsValid() {
			tid = sc.TraceID().String()
		}
		if tid == "" {
			tid = c.GetHeader(TraceIDHeader)
		}
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
