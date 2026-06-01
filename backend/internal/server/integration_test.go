package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// TestAuthFlowIntegration drives the real HTTP stack (router + middleware +
// modules) against a real Postgres, exercising the full auth lifecycle
// including H4 refresh-token rotation/reuse/logout. It is skipped unless
// DATABASE_URL is set, so the default `go test ./...` (no DB) stays green; CI
// provides a Postgres service and sets DATABASE_URL.
func TestAuthFlowIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Env:             "test",
		JWTSecret:       "integration-secret",
		JWTAccessTTL:    15 * time.Minute,
		JWTRefreshTTL:   time.Hour,
		PIISecret:       "integration-pii",
		KYCAutoApprove:  true,
		RedisURL:        "redis://127.0.0.1:1/0", // unreachable → in-memory limiter + denylist
		StorageDriver:   "local",
		StorageDir:      t.TempDir(),
		CORSAllowOrigin: "*",
		PaymentProvider: "mock",
	}
	srv := New(cfg, pool)

	// Unique account so the test is safe to re-run against the same DB.
	account := fmt.Sprintf("itest_%d@example.com", time.Now().UnixNano())

	type authData struct {
		User struct {
			Account string `json:"account"`
		} `json:"user"`
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	do := func(method, path, bearer string, body any) (int, json.RawMessage) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatalf("encode body: %v", err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		var env struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, env.Data
	}
	decode := func(raw json.RawMessage) authData {
		var d authData
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("decode auth data: %v", err)
		}
		return d
	}

	// 1) Register.
	st, raw := do(http.MethodPost, "/api/v1/auth/register", "",
		map[string]string{"account": account, "account_type": "email", "password": "password123"})
	if st != http.StatusOK {
		t.Fatalf("register status = %d, want 200 (body=%s)", st, raw)
	}
	reg := decode(raw)
	if reg.User.Account != account || reg.Tokens.AccessToken == "" || reg.Tokens.RefreshToken == "" {
		t.Fatalf("register returned unexpected data: %+v", reg)
	}

	// 2) GET /users/me with the access token.
	st, raw = do(http.MethodGet, "/api/v1/users/me", reg.Tokens.AccessToken, nil)
	if st != http.StatusOK {
		t.Fatalf("me status = %d, want 200 (body=%s)", st, raw)
	}

	// 2b) /users/me without a token → 401.
	if st, _ := do(http.MethodGet, "/api/v1/users/me", "", nil); st != http.StatusUnauthorized {
		t.Fatalf("me without token status = %d, want 401", st)
	}

	// 3) Refresh rotates the refresh token.
	st, raw = do(http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": reg.Tokens.RefreshToken})
	if st != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200 (body=%s)", st, raw)
	}
	rotated := decode(raw)
	if rotated.Tokens.RefreshToken == reg.Tokens.RefreshToken {
		t.Fatal("refresh did not rotate the token")
	}

	// 4) Reusing the original (now-rotated) refresh token → 401.
	if st, _ := do(http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": reg.Tokens.RefreshToken}); st != http.StatusUnauthorized {
		t.Fatalf("reused refresh status = %d, want 401", st)
	}

	// 5) Logout the rotated token, then it can no longer refresh.
	if st, _ := do(http.MethodPost, "/api/v1/auth/logout", "",
		map[string]string{"refresh_token": rotated.Tokens.RefreshToken}); st != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", st)
	}
	if st, _ := do(http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": rotated.Tokens.RefreshToken}); st != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401", st)
	}
}
