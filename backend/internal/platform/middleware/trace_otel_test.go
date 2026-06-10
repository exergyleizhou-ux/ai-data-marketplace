package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// When an OTel span is active on the request context (recording provider),
// trace_id must be the span's trace ID so logs and traces correlate.
func TestTraceID_UsesActiveSpanTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	var gotTID string
	var wantTID string
	r := gin.New()
	// Simulate otelgin: start a real span before TraceID runs.
	r.Use(func(c *gin.Context) {
		ctx, span := tp.Tracer("test").Start(c.Request.Context(), "HTTP GET /x")
		defer span.End()
		wantTID = span.SpanContext().TraceID().String()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(TraceID())
	r.GET("/x", func(c *gin.Context) {
		gotTID = c.GetString("trace_id")
		c.Status(200)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))

	if gotTID == "" || gotTID != wantTID {
		t.Fatalf("trace_id = %q, want active span trace id %q", gotTID, wantTID)
	}
	if h := w.Header().Get(TraceIDHeader); h != wantTID {
		t.Fatalf("X-Trace-ID header = %q, want %q", h, wantTID)
	}
}

// Without a recording span (no-op provider — the default when OTel is
// disabled), TraceID must keep its original behavior: random 32-hex ID.
func TestTraceID_FallsBackWithoutSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var tid string
	r := gin.New()
	r.Use(TraceID())
	r.GET("/x", func(c *gin.Context) {
		tid = c.GetString("trace_id")
		// The context must NOT carry a valid OTel span context here.
		if oteltrace.SpanContextFromContext(c.Request.Context()).IsValid() {
			t.Error("no span expected on a bare request")
		}
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if len(tid) != 32 {
		t.Fatalf("fallback trace_id length = %d, want 32", len(tid))
	}
}
