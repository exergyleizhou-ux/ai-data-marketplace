package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
)

func getMetrics(s *Server, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// Regression coverage for the /metrics bearer-token gate: when METRICS_TOKEN is
// set, only the exact token is accepted; otherwise the endpoint is open.
func TestMetricsAuth_EnforcesToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New(&config.Config{Env: "test", MetricsToken: "s3cret-scrape-token"}, nil)

	if rec := getMetrics(s, "Bearer s3cret-scrape-token"); rec.Code != http.StatusOK {
		t.Fatalf("correct token: status %d, want 200", rec.Code)
	}
	if rec := getMetrics(s, "Bearer wrong-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want 401", rec.Code)
	}
	if rec := getMetrics(s, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status %d, want 401", rec.Code)
	}
	// A token that is a prefix of the real one must not be accepted.
	if rec := getMetrics(s, "Bearer s3cret"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("prefix token: status %d, want 401", rec.Code)
	}
}

func TestMetricsAuth_OpenWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New(&config.Config{Env: "test"}, nil) // METRICS_TOKEN unset

	if rec := getMetrics(s, ""); rec.Code != http.StatusOK {
		t.Fatalf("metrics must be open when token unset: status %d, want 200", rec.Code)
	}
}
