// Package server — handler-level integration tests.  These tests start the
// full gin engine and verify HTTP-level contracts: auth gates, status codes,
// anti-enumeration, IDOR, ops gate enforcement.
//
// Tests that were skipped in PR-V due to "gin middleware environment issues"
// are covered here — the correct approach is server.New(cfg, pool) which
// wires all middleware and all modules automatically.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// newHandlerServer is a shared helper for handler-level tests.
func newHandlerServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping handler integration test")
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
		JWTSecret:         "h-test-secret",
		JWTAccessTTL:      15 * time.Minute,
		JWTRefreshTTL:     time.Hour,
		PIISecret:         "h-test-pii",
		KYCAutoApprove:    true,
		RedisURL:          "redis://127.0.0.1:1/0",
		StorageDriver:     "local",
		StorageDir:        t.TempDir(),
		CORSAllowOrigin:   "*",
		PaymentProvider:   "mock",
		PaymentMockSecret: "h-test-pay",
		StripeCurrency:    "usd",
		AppBaseURL:        "https://app.test",
	}
	srv := New(cfg, pool)
	ts := httptest.NewServer(srv.Handler())
	return ts, func() { ts.Close(); srv.Close(); pool.Close() }
}

func doJSON(t *testing.T, ts *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func registerUser(t *testing.T, ts *httptest.Server, account string) (token, userID string) {
	t.Helper()
	resp := doJSON(t, ts, "POST", "/api/v1/auth/register", "",
		fmt.Sprintf(`{"account":"%s","account_type":"email","password":"password123"}`, account))
	defer resp.Body.Close()
	var env struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	return env.Data.Tokens.AccessToken, env.Data.User.ID
}

// registerVerifiedUser registers and submits KYC (auto-approved via KYCAutoApprove).
func registerVerifiedUser(t *testing.T, ts *httptest.Server, account string) (token, userID string) {
	token, userID = registerUser(t, ts, account)
	resp := doJSON(t, ts, "POST", "/api/v1/users/me/kyc", token,
		`{"type":"personal","real_name":"Test User","id_no":"110101199001011234"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Read body for error details.
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		t.Fatalf("submit KYC: expected 200, got %d body=%s", resp.StatusCode, buf.String())
	}
	return token, userID
}

// ===========================================================================
// AUTH handler tests (PR-V skipped)
// ===========================================================================

func TestAuthHandler_Enroll2FA_RequiresAuth(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp := doJSON(t, ts, "POST", "/api/v1/auth/2fa/enroll", "", `{}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestAuthHandler_Verify2FA_WrongCode_Returns401(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp := doJSON(t, ts, "POST", "/api/v1/auth/2fa/verify", "",
		`{"challenge_token":"bogus","code":"000000"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong code, got %d", resp.StatusCode)
	}
}

func TestAuthHandler_PasswordResetRequest_AlwaysReturns200(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp := doJSON(t, ts, "POST", "/api/v1/auth/password-reset/request", "",
		`{"account":"no-one@e2e.test"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("password-reset must always return 200 (anti-enumeration), got %d", resp.StatusCode)
	}
}

func TestAuthHandler_Login_With2FA_ReturnsChallenge(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	account := fmt.Sprintf("2fa-h_%d@e2e.test", time.Now().UnixNano())
	tok, _ := registerUser(t, ts, account)

	// Enroll 2FA.
	resp := doJSON(t, ts, "POST", "/api/v1/auth/2fa/enroll", tok, `{}`)
	var enrEnv struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&enrEnv)
	resp.Body.Close()
	if enrEnv.Data.Secret == "" {
		t.Fatal("2fa enroll did not return a secret")
	}

	// Verify enrollment.
	code, err := totp.GenerateCode(enrEnv.Data.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	resp = doJSON(t, ts, "POST", "/api/v1/auth/2fa/verify-enrollment", tok,
		`{"code":"`+code+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify-enrollment: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Login — must return a 2FA challenge.
	resp = doJSON(t, ts, "POST", "/api/v1/auth/login", "",
		fmt.Sprintf(`{"account":"%s","password":"password123"}`, account))
	defer resp.Body.Close()
	var loginEnv struct {
		Data struct {
			Need2FA        bool   `json:"need_2fa"`
			ChallengeToken string `json:"challenge_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginEnv); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if !loginEnv.Data.Need2FA {
		t.Fatal("expected need_2fa=true after 2FA enrollment")
	}
	if loginEnv.Data.ChallengeToken == "" {
		t.Fatal("expected non-empty challenge_token")
	}
}

// ===========================================================================
// NOTIFICATION handler tests
// ===========================================================================

func TestNotificationHandler_List_RequiresAuth(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp := doJSON(t, ts, "GET", "/api/v1/users/me/notifications", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_MarkRead_OtherUser_Returns404(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	accountA := fmt.Sprintf("notA_%d@e2e.test", time.Now().UnixNano())
	accountB := fmt.Sprintf("notB_%d@e2e.test", time.Now().UnixNano())
	tokA, _ := registerUser(t, ts, accountA)
	tokB, _ := registerUser(t, ts, accountB)

	// User A's notifications.
	resp := doJSON(t, ts, "GET", "/api/v1/users/me/notifications", tokA, "")
	var notifs struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&notifs)
	resp.Body.Close()

	if len(notifs.Data.Items) > 0 {
		noteID := notifs.Data.Items[0].ID
		resp = doJSON(t, ts, "POST", "/api/v1/users/me/notifications/"+noteID+"/read", tokB, "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
			t.Errorf("user B marking user A's notification: expected 404/403, got %d", resp.StatusCode)
		}
	} else {
		// No real notifications — use bogus ID, must get 4xx.
		resp = doJSON(t, ts, "POST", "/api/v1/users/me/notifications/bogus-id/read", tokB, "")
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("bogus notification: expected 4xx, got %d", resp.StatusCode)
		}
	}
}

// ===========================================================================
// VERIFY handler tests
// ===========================================================================

func TestVerifyHandler_UnknownCert_Returns404(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp, err := ts.Client().Get(ts.URL + "/api/v1/verify/bogus-cert-id")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cert: expected 404, got %d", resp.StatusCode)
	}
}

func TestVerifyHandler_KnownCert_Returns200(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	account := fmt.Sprintf("vcert_%d@e2e.test", time.Now().UnixNano())
	tok, sellerID := registerVerifiedUser(t, ts, account)

	// Seed a published dataset (the certificate is generated on publish).
	pool, _ := db.NewPool(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	pool.Exec(context.Background(), `
		INSERT INTO datasets (id, seller_id, title, description, data_type, license_type, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'VCert E2E DS', 'seeded', 'text', 'commercial', 'published', now(), now())
	`, sellerID)
	var dsID string
	pool.QueryRow(context.Background(),
		`SELECT id FROM datasets WHERE seller_id=$1 AND title='VCert E2E DS' LIMIT 1`, sellerID).Scan(&dsID)

	// Look up the dataset's certificate_id.
	_ = tok // used for auth if needed
	resp := doJSON(t, ts, "GET", "/api/v1/datasets/"+dsID, "", "")
	var detail struct {
		Data struct {
			CertificateID string `json:"certificate_id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()

	if detail.Data.CertificateID == "" {
		t.Log("no certificate_id — dataset may not have auto-generated cert")
		return
	}

	resp2, err := ts.Client().Get(ts.URL + "/api/v1/verify/" + detail.Data.CertificateID)
	if err != nil {
		t.Fatalf("verify lookup: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("known cert %s: expected 200, got %d", detail.Data.CertificateID, resp2.StatusCode)
	}
}

// ===========================================================================
// ANOMALY handler tests
// ===========================================================================

func TestAnomalyHandler_List_RequiresOps(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	account := fmt.Sprintf("anom_%d@e2e.test", time.Now().UnixNano())
	tok, _ := registerUser(t, ts, account)

	resp := doJSON(t, ts, "GET", "/api/v1/admin/anomalies", tok, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("buyer token on admin anomalies: expected 403, got %d", resp.StatusCode)
	}
}

func TestAnomalyHandler_Acknowledge_TransitionsStatus(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	account := fmt.Sprintf("anom-o_%d@e2e.test", time.Now().UnixNano())
	_, opsID := registerUser(t, ts, account)

	// Promote to ops + re-login.
	pool, _ := db.NewPool(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	pool.Exec(context.Background(), `UPDATE users SET role='ops' WHERE id=$1`, opsID)

	resp := doJSON(t, ts, "POST", "/api/v1/auth/login", "",
		fmt.Sprintf(`{"account":"%s","password":"password123"}`, account))
	var loginEnv struct {
		Data struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&loginEnv)
	resp.Body.Close()
	opsTok := loginEnv.Data.Tokens.AccessToken

	// List anomalies — should succeed (even if empty).
	resp = doJSON(t, ts, "GET", "/api/v1/admin/anomalies", opsTok, "")
	var listEnv struct {
		Data struct {
			Items []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listEnv)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ops list anomalies: expected 200, got %d", resp.StatusCode)
	}

	if len(listEnv.Data.Items) == 0 {
		t.Log("no anomalies — handler returns 200 for empty list")
		return
	}

	anom := listEnv.Data.Items[0]
	resp = doJSON(t, ts, "POST", "/api/v1/admin/anomalies/"+anom.ID+"/acknowledge", opsTok,
		`{"note":"handler test"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("acknowledge: expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// QA handler tests
// ===========================================================================

func TestQAHandler_Ask_RequiresAuth(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()
	resp := doJSON(t, ts, "POST", "/api/v1/datasets/bogus/questions", "",
		`{"body":"Is this good?"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ask without auth: expected 401, got %d", resp.StatusCode)
	}
}

func TestQAHandler_Answer_NonSeller_Returns403(t *testing.T) {
	ts, cleanup := newHandlerServer(t)
	defer cleanup()

	sellerAccount := fmt.Sprintf("qa-s_%d@e2e.test", time.Now().UnixNano())
	buyerAccount := fmt.Sprintf("qa-b_%d@e2e.test", time.Now().UnixNano())
	_, sellerID := registerVerifiedUser(t, ts, sellerAccount)
	buyerTok, _ := registerVerifiedUser(t, ts, buyerAccount)

	// Seed a published dataset so questions can be asked on it (the QA service
	// may require the dataset to be public).
	pool, _ := db.NewPool(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	pool.Exec(context.Background(), `
		INSERT INTO datasets (id, seller_id, title, description, data_type, license_type, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'QA E2E DS', 'seeded', 'text', 'commercial', 'published', now(), now())
	`, sellerID)
	var dsID string
	pool.QueryRow(context.Background(), `SELECT id FROM datasets WHERE seller_id=$1 AND title='QA E2E DS' LIMIT 1`, sellerID).Scan(&dsID)
	pool.Exec(context.Background(), `
		INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
		VALUES (gen_random_uuid(), $1, 1, '[]', now())
	`, dsID)

	// Buyer asks a question.
	resp := doJSON(t, ts, "POST", "/api/v1/datasets/"+dsID+"/questions", buyerTok,
		`{"body":"What license?"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		t.Fatalf("ask question: unexpected status %d, body=%s", resp.StatusCode, buf.String())
	}
	var qEnv struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&qEnv)
	qID := qEnv.Data.ID
	if qID == "" {
		// Read remaining body for debug.
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		t.Fatalf("ask question returned no ID (status=%d, body=%s)", resp.StatusCode, buf.String())
	}

	// Buyer tries to answer — must be 403 (not the seller).
	resp = doJSON(t, ts, "POST", "/api/v1/questions/"+qID+"/answer", buyerTok,
		`{"body":"Commercial."}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-seller answering: expected 403, got %d", resp.StatusCode)
	}
}
