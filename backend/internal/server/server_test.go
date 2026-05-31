package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

func newTestServer() *Server {
	gin.SetMode(gin.TestMode)
	// db is nil: readyz skips the DB ping, healthz and /api/v1 routes don't touch it.
	return New(&config.Config{Env: "test"}, nil)
}

func TestMetrics(t *testing.T) {
	srv := newTestServer()
	// Generate one request so a counter sample exists.
	srv.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"marketplace_http_requests_total", "marketplace_http_request_duration_seconds", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
}

func TestPingEnvelope(t *testing.T) {
	srv := newTestServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ping status = %d, want 200", rec.Code)
	}

	var body httpx.Body
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != 0 {
		t.Errorf("body.Code = %d, want 0", body.Code)
	}
	if body.Message != "ok" {
		t.Errorf("body.Message = %q, want %q", body.Message, "ok")
	}
	// RequestID middleware must populate the correlation id and echo the header.
	if body.RequestID == "" {
		t.Error("body.RequestID is empty, want a generated id")
	}
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("response missing X-Request-ID header")
	}
}
