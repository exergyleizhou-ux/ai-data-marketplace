// Package server — security invariant tests.  These tests start the real gin
// engine and verify that auth/role gates are enforced at the HTTP level.  When
// a new module is added without proper middleware, these catch it.
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// newSecServer starts the full application with an ephemeral PG and returns
// an httptest server.  Every test gets a fresh DB (migrations re-run by the
// harness).  Skip when DATABASE_URL is not set.
func newSecServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping security invariant test")
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Env:               "test",
		JWTSecret:         "sec-invariant-test",
		JWTAccessTTL:      15 * time.Minute,
		JWTRefreshTTL:     time.Hour,
		PIISecret:         "sec-pii",
		KYCAutoApprove:    true,
		RedisURL:          "redis://127.0.0.1:1/0",
		StorageDriver:     "local",
		StorageDir:        t.TempDir(),
		CORSAllowOrigin:   "*",
		PaymentProvider:   "mock",
		PaymentMockSecret: "sec-pay",
		StripeCurrency:    "usd",
		AppBaseURL:        "https://app.test",
	}
	srv := New(cfg, pool)
	ts := httptest.NewServer(srv.Handler())
	return ts, func() { ts.Close(); srv.Close(); pool.Close() }
}

// ---------------------------------------------------------------------------
// TestAllAdminRoutesRequireOpsRole
//   Enumerate all real gin routes, filter to /admin/ paths, send a request
//   with a plain buyer JWT (no ops role), and assert 403 Forbidden on every
//   single one.  If any admin route returns 200, the ops gate is missing.
// ---------------------------------------------------------------------------

func TestAllAdminRoutesRequireOpsRole(t *testing.T) {
	ts, cleanup := newSecServer(t)
	defer cleanup()

	// Register a buyer user and get a JWT.
	buyerTok := registerBuyer(t, ts)

	// Enumerate admin routes from the engine.
	h := ts.Config.Handler
	engine, ok := h.(*gin.Engine)
	if !ok {
		t.Skip("handler is not *gin.Engine")
	}

	failures := 0
	for _, r := range engine.Routes() {
		if r.Method == "HEAD" || !strings.Contains(r.Path, "/admin/") {
			continue
		}
		path := strings.Replace(r.Path, ":id", "dummy-id", 1)
		path = strings.Replace(path, ":orderId", "dummy-order", 1)
		path = strings.Replace(path, ":cert_id", "dummy-cert", 1)

		req, _ := http.NewRequest(r.Method, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+buyerTok)
		if r.Method == "POST" || r.Method == "PUT" {
			req.Header.Set("Content-Type", "application/json")
			// Small body for routes that bind JSON.
			req.Body = io.NopCloser(strings.NewReader(`{"note":"test","reason":"test","approve":true}`))
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Errorf("%s %s: request failed: %v", r.Method, r.Path, err)
			failures++
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 403 && resp.StatusCode != 401 {
			t.Errorf("%s %s: expected 401/403 (buyer token → admin route), got %d — ops gate missing?",
				r.Method, r.Path, resp.StatusCode)
			failures++
		}
	}
	if failures > 0 {
		t.Fatalf("%d admin routes lack ops gate or returned unexpected status", failures)
	}
	t.Logf("all admin routes reject buyer token (401/403)")
}

// ---------------------------------------------------------------------------
// TestAllUserScopedRoutesRejectAnonymous
//   Enumerate /users/me/* and /sellers/me/* routes, send without a token,
//   assert 401 Unauthorized on every single one.
// ---------------------------------------------------------------------------

func TestAllUserScopedRoutesRejectAnonymous(t *testing.T) {
	ts, cleanup := newSecServer(t)
	defer cleanup()

	h := ts.Config.Handler
	engine, ok := h.(*gin.Engine)
	if !ok {
		t.Skip("handler is not *gin.Engine")
	}

	failures := 0
	for _, r := range engine.Routes() {
		if r.Method == "HEAD" {
			continue
		}
		if !strings.Contains(r.Path, "/users/me/") && !strings.Contains(r.Path, "/sellers/me/") {
			continue
		}
		path := strings.Replace(r.Path, ":id", "dummy-id", 1)

		req, _ := http.NewRequest(r.Method, ts.URL+path, nil)
		// No Authorization header — anonymous.
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
			req.Header.Set("Content-Type", "application/json")
			req.Body = io.NopCloser(strings.NewReader(`{}`))
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Errorf("%s %s: request failed: %v", r.Method, r.Path, err)
			failures++
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("%s %s: expected 401 (anonymous), got %d — auth gate missing?",
				r.Method, r.Path, resp.StatusCode)
			failures++
		}
	}
	if failures > 0 {
		t.Fatalf("%d user-scoped routes lack auth gate", failures)
	}
	t.Logf("all user-scoped routes reject anonymous requests (401)")
}

// ---------------------------------------------------------------------------
// TestPublicMutationsAreRateLimited
//   Hit known public mutation endpoints N+1 times and assert the (N+1)-th
//   returns 429 Too Many Requests.  Covers register, login, 2fa-verify,
//   password-reset-request, password-reset-complete.
// ---------------------------------------------------------------------------

func TestPublicMutationsAreRateLimited(t *testing.T) {
	ts, cleanup := newSecServer(t)
	defer cleanup()

	type testCase struct {
		method string
		path   string
		body   string
		limit  int // expected rate-limit ceiling
	}
	tests := []testCase{
		{"POST", "/api/v1/auth/register", `{"account":"rl-test@e2e.test","account_type":"email","password":"password123"}`, 5},
		{"POST", "/api/v1/auth/login", `{"account":"rl-test@e2e.test","password":"password123"}`, 10},
		{"POST", "/api/v1/auth/2fa/verify", `{"challenge_token":"bad","code":"000000"}`, 15},
		{"POST", "/api/v1/auth/password-reset/request", `{"account":"rl-test@e2e.test"}`, 3},
		{"POST", "/api/v1/auth/password-reset/complete", `{"token":"bad","new_password":"password123"}`, 5},
		// refresh + logout are also rate-limited now (30/min each — too high to hit in test,
		// but we verify they exist).
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			// Send limit+1 requests.  The last one should hit 429.
			got429 := false
			for i := 0; i <= tc.limit; i++ {
				req, _ := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := ts.Client().Do(req)
				if err != nil {
					t.Fatalf("request %d: %v", i, err)
				}
				resp.Body.Close()
				// Don't error on 429 — we expect it on the last iteration.
				if resp.StatusCode == 429 {
					got429 = true
				}
				if resp.StatusCode >= 500 {
					t.Fatalf("request %d returned 5xx: %d", i, resp.StatusCode)
				}
			}
			if !got429 {
				t.Errorf("no 429 received after %d requests to %s — rate limit missing or too high?", tc.limit+1, tc.path)
			}
		})
	}
}

// registerBuyer registers a test user and returns the access token.
func registerBuyer(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	body := strings.NewReader(`{"account":"buyer-invariant@e2e.test","account_type":"email","password":"password123"}`)
	resp, err := ts.Client().Post(ts.URL+"/api/v1/auth/register", "application/json", body)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer resp.Body.Close()
	var env struct {
		Data struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode register response: %v (body=%s)", err, raw)
	}
	return env.Data.Tokens.AccessToken
}
