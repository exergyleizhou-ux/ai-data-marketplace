package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceID_GeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/", func(c *gin.Context) {
		tid, _ := c.Get("trace_id")
		c.String(200, tid.(string))
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	tid := w.Header().Get(TraceIDHeader)
	if tid == "" {
		t.Fatal("X-Trace-ID header must be set")
	}
	if len(tid) != 32 {
		t.Fatalf("trace_id length = %d, want 32 (16-byte hex)", len(tid))
	}
}

func TestTraceID_PassesThroughWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/", func(c *gin.Context) {
		tid, _ := c.Get("trace_id")
		c.String(200, tid.(string))
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(TraceIDHeader, "my-custom-trace-id")
	r.ServeHTTP(w, req)

	if w.Header().Get(TraceIDHeader) != "my-custom-trace-id" {
		t.Fatalf("X-Trace-ID = %q, want my-custom-trace-id", w.Header().Get(TraceIDHeader))
	}
}

func TestTraceID_PutsIntoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/", func(c *gin.Context) {
		tid := TraceIDFromContext(c.Request.Context())
		if tid == "" {
			c.String(http.StatusInternalServerError, "missing")
			return
		}
		c.String(http.StatusOK, tid)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatal("TraceIDFromContext returned empty string")
	}
}
