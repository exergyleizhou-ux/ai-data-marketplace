package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)

	r := gin.New()
	r.Use(Middleware(tm))
	r.GET("/protected", func(c *gin.Context) {
		httpx.OK(c, gin.H{"uid": httpx.UserID(c), "role": httpx.UserRole(c)})
	})

	// No token -> 401.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", rec.Code)
	}

	// Valid token -> 200 and identity is injected.
	tokens, err := tm.Issue("user-42", "seller")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: status = %d, want 200", rec.Code)
	}

	// A refresh token must not authorize access -> 401.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.RefreshToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token as access: status = %d, want 401", rec.Code)
	}
}
