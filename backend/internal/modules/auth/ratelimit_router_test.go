package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// TestRegisterRouteRateLimited verifies the rate-limit middleware is actually
// wired onto /auth/register (limit 5/min): the 6th request from one IP gets 429
// before reaching the handler. Guards against the limiter being dropped from
// the route in a future refactor.
func TestRegisterRouteRateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	svc := NewService(newFakeRepo(), tm)

	r := gin.New()
	Register(r.Group("/api/v1"), svc, tm, ratelimit.NewInMemory())

	codes := make([]int, 0, 6)
	for i := 0; i < 6; i++ {
		body := fmt.Sprintf(`{"account":"rl%d@x.com","account_type":"email","password":"Password123!"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.9:5555" // stable client IP → same bucket
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}

	// First 5 pass the limiter (limit=5); the 6th is throttled.
	if codes[5] != http.StatusTooManyRequests {
		t.Fatalf("6th /auth/register: got %d, want 429 (rate limit not wired?). all=%v", codes[5], codes)
	}
	for i := 0; i < 5; i++ {
		if codes[i] == http.StatusTooManyRequests {
			t.Fatalf("request %d throttled too early: %v", i+1, codes)
		}
	}
}
