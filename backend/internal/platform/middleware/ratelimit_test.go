package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(ratelimit.NewInMemory(), RateLimitConfig{
		Name: "test", Limit: 2, Window: time.Minute,
	}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "203.0.113.7:1234" // stable client IP
		r.ServeHTTP(rec, req)
		return rec
	}

	if c := do().Code; c != http.StatusOK {
		t.Fatalf("req 1 = %d, want 200", c)
	}
	if c := do().Code; c != http.StatusOK {
		t.Fatalf("req 2 = %d, want 200", c)
	}
	rec := do()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("req 3 = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response missing Retry-After header")
	}
}
