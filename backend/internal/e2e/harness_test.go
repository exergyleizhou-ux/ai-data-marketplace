// Package e2e holds full-stack HTTP end-to-end tests that run against a real
// Postgres + complete gin router (all 16 modules wired).  Each test exercises a
// cross-module business journey — the kind of regression net that module-level
// tests cannot provide.
//
// Tests are skipped when DATABASE_URL is not set, matching the convention in
// internal/server/integration_test.go.  CI provides a Postgres service and
// sets DATABASE_URL.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/server"
)

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

// e2eEnv holds a running application server and convenience helpers.  Call
// t.Cleanup(e.Close) or defer e.Close() to drain background goroutines.
type e2eEnv struct {
	t      *testing.T
	srv    *server.Server
	ts     *httptest.Server
	client *http.Client
	dbDSN  string
	pool   interface {
		Exec(context.Context, string, ...any) (interface{}, error)
	} // pgxpool.Pool — used for seed queries
}

func newE2E(t *testing.T) *e2eEnv {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping full-stack E2E test")
	}

	// Fresh schema on every test run.
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Env:               "test",
		JWTSecret:         "e2e-secret",
		JWTAccessTTL:      15 * time.Minute,
		JWTRefreshTTL:     time.Hour,
		PIISecret:         "e2e-pii",
		KYCAutoApprove:    true,
		RedisURL:          "redis://127.0.0.1:1/0", // unreachable → in-memory limiter
		StorageDriver:     "local",
		StorageDir:        t.TempDir(),
		CORSAllowOrigin:   "*",
		PaymentProvider:   "mock",
		PaymentMockSecret: "e2e-pay-secret",
		StripeCurrency:    "usd",
		AppBaseURL:        "https://app.test",
	}

	srv := server.New(cfg, pool)
	ts := httptest.NewServer(srv.Handler())

	e := &e2eEnv{
		t:      t,
		srv:    srv,
		ts:     ts,
		client: ts.Client(),
		dbDSN:  dsn,
	}

	t.Cleanup(func() {
		ts.Close()
		srv.Close()
		pool.Close()
	})
	return e
}

// post sends a POST to the running server.  bearer may be empty.
func (e *e2eEnv) post(path string, body any, bearer string) *e2eResp {
	return e.do("POST", path, body, bearer)
}

// get sends a GET.
func (e *e2eEnv) get(path string, bearer string) *e2eResp {
	return e.do("GET", path, nil, bearer)
}

func (e *e2eEnv) do(method, path string, body any, bearer string) *e2eResp {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, r)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return &e2eResp{status: resp.StatusCode, raw: raw}
}

type e2eResp struct {
	status int
	raw    []byte
}

// ok asserts status 200 and unmarshals the "data" envelope into v.
func (r *e2eResp) ok(t *testing.T, v any) {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", r.status, string(r.raw))
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.raw, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, string(r.raw))
	}
	if v != nil && env.Data != nil {
		if err := json.Unmarshal(env.Data, v); err != nil {
			t.Fatalf("decode data: %v (data=%s)", err, string(env.Data))
		}
	}
}

// code asserts status and unmarshals data.
func (r *e2eResp) code(t *testing.T, want int, v any) {
	t.Helper()
	if r.status != want {
		t.Fatalf("expected %d, got %d: %s", want, r.status, string(r.raw))
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.raw, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, string(r.raw))
	}
	if v != nil && env.Data != nil {
		if err := json.Unmarshal(env.Data, v); err != nil {
			t.Fatalf("decode data: %v (data=%s)", err, string(env.Data))
		}
	}
}

// body returns the raw body string.
func (r *e2eResp) body() string { return string(r.raw) }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type e2eAuthResult struct {
	User struct {
		ID      string `json:"id"`
		Account string `json:"account"`
		Role    string `json:"role"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"tokens"`
}

type e2eLoginResult struct {
	Need2FA        bool            `json:"need_2fa"`
	ChallengeToken string          `json:"challenge_token"`
	User           json.RawMessage `json:"user"`
	Tokens         json.RawMessage `json:"tokens"`
}

type e2eEnroll2FAResult struct {
	Secret        string   `json:"secret"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// registerAndLogin is a shortcut that registers a user, optionally enables 2FA
// and completes enrollment, then returns the access token + user id.
func (e *e2eEnv) registerAndLogin(account, password string) (accessToken, userID string) {
	e.t.Helper()

	var ar e2eAuthResult
	e.post("/api/v1/auth/register", map[string]string{
		"account":      account,
		"account_type": "email",
		"password":     password,
	}, "").ok(e.t, &ar)
	return ar.Tokens.AccessToken, ar.User.ID
}

// uniqueAccount returns a test-unique email address.
func uniqueAccount(prefix string) string {
	return fmt.Sprintf("%s_%d@e2e.test", prefix, time.Now().UnixNano())
}

// seedQuery runs a raw SQL statement against the test database.
func (e *e2eEnv) seedQuery(t *testing.T, query string, args ...any) {
	t.Helper()
	pool, err := db.NewPool(context.Background(), e.dbDSN)
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("seed query: %v (sql=%s)", err, query)
	}
}

// Ensure types are available (avoids unused import lint).
var _ = server.New
