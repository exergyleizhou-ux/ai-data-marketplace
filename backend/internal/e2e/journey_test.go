package e2e

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pquerna/otp/totp"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/payment"
	"github.com/lei/ai-data-marketplace/backend/migrations"
)

// ---------------------------------------------------------------------------
// Scenario 1: Full purchase journey (cross-module: auth → dataset → order → payment)
// ---------------------------------------------------------------------------

func TestE2E_FullPurchaseJourney(t *testing.T) {
	e := newE2E(t)

	_, sellerID := e.registerAndLogin(uniqueAccount("pseller"), "password123")
	buyerTok, buyerID := e.registerAndLogin(uniqueAccount("pbuyer"), "password123")

	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)
	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, buyerID)

	// SIMPLIFICATION: seed dataset+version in published state directly.
	// The dataset create API is tested by the dataset module's own integration
	// tests; E2E focuses on the cross-module order→payment→delivery contract.
	e.seedQuery(t, `
		INSERT INTO datasets (id, seller_id, title, description, data_type, license_type, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'E2E Purchase DS', 'Seeded for E2E', 'text', 'commercial', 'published', now(), now())
	`, sellerID)
	var datasetID string
	e.seedQueryRow(t, []any{&datasetID},
		`SELECT id FROM datasets WHERE seller_id=$1 AND title='E2E Purchase DS' LIMIT 1`, sellerID)

	e.seedQuery(t, `
		INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
		VALUES (gen_random_uuid(), $1, 1, '[]', now())
	`, datasetID)
	var verID string
	e.seedQueryRow(t, []any{&verID},
		`SELECT id FROM dataset_versions WHERE dataset_id=$1 LIMIT 1`, datasetID)
	e.seedQuery(t, `
		UPDATE datasets SET current_version_id=$1 WHERE id=$2
	`, verID, datasetID)

	// Buyer browses datasets.
	var browseRes struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	e.get("/api/v1/datasets", buyerTok).ok(t, &browseRes)
	found := false
	for _, it := range browseRes.Items {
		if it.ID == datasetID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("published dataset %s not visible in browse", datasetID)
	}

	// Buyer creates order.
	type orderRes struct {
		ID       string `json:"id"`
		BuyerID  string `json:"buyer_id"`
		SellerID string `json:"seller_id"`
		Status   string `json:"status"`
	}
	var ord orderRes
	e.post("/api/v1/orders", map[string]any{
		"dataset_id":   datasetID,
		"license_type": "commercial",
	}, buyerTok).ok(t, &ord)
	if ord.Status != "created" {
		t.Fatalf("order status after create = %q, want created", ord.Status)
	}

	// Buyer initiates payment.
	type payInfo struct {
		ChannelTxnID string `json:"channel_txn_id"`
	}
	var pi payInfo
	e.post("/api/v1/payments/create", map[string]any{"order_id": ord.ID}, buyerTok).ok(t, &pi)
	if pi.ChannelTxnID == "" {
		t.Fatalf("expected channel_txn_id from /payments/create")
	}

	// Mock provider webhook: payload format is "<order_id>:<channel_txn_id>:true",
	// signature is HMAC-SHA256 hex with the PaymentMockSecret ("e2e-pay-secret").
	payload := []byte(fmt.Sprintf("%s:%s:true", ord.ID, pi.ChannelTxnID))
	sig := payment.Sign("e2e-pay-secret", string(payload))
	res := e.postWebhook(t, "/api/v1/payments/webhook/mock", payload, sig)
	if res.status != 200 {
		t.Fatalf("webhook status=%d body=%s", res.status, res.body())
	}

	// Poll order status: should reach "paid".
	deadline := time.Now().Add(3 * time.Second)
	var final orderRes
	for time.Now().Before(deadline) {
		e.get("/api/v1/orders/"+ord.ID, buyerTok).ok(t, &final)
		if final.Status == "paid" || final.Status == "delivered" || final.Status == "confirmed" || final.Status == "settled" {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if final.Status != "paid" {
		t.Fatalf("order status after webhook = %q, want paid", final.Status)
	}

	// Seed to delivered so confirm-delivery can run (delivery flow has its own
	// module-level integration tests for the token issuance / file-staging steps).
	e.seedQuery(t, `UPDATE orders SET status='delivered' WHERE id=$1`, ord.ID)

	e.post("/api/v1/orders/"+ord.ID+"/confirm-delivery", nil, buyerTok).ok(t, &final)
	if final.Status != "settled" && final.Status != "confirmed" {
		t.Fatalf("order status after confirm = %q, want settled or confirmed", final.Status)
	}

	// Review.
	res2 := e.post("/api/v1/orders/"+ord.ID+"/review",
		map[string]any{"score": 5, "comment": "great dataset"}, buyerTok)
	if res2.status != 200 {
		t.Fatalf("review status=%d body=%s", res2.status, res2.body())
	}
	t.Logf("full purchase journey: dataset=%s order=%s final=%s", datasetID, ord.ID, final.Status)
}

// ---------------------------------------------------------------------------
// Scenario 2: 2FA login full flow (PR-V end-to-end verification)
// ---------------------------------------------------------------------------

func TestE2E_TwoFactorLoginFlow(t *testing.T) {
	e := newE2E(t)
	account := uniqueAccount("2fa")

	var ar e2eAuthResult
	e.post("/api/v1/auth/register", map[string]string{
		"account":      account,
		"account_type": "email",
		"password":     "password123",
	}, "").ok(t, &ar)
	tok := ar.Tokens.AccessToken

	var enr e2eEnroll2FAResult
	e.post("/api/v1/auth/2fa/enroll", nil, tok).ok(t, &enr)
	if len(enr.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(enr.RecoveryCodes))
	}

	code, err := totp.GenerateCode(enr.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	e.post("/api/v1/auth/2fa/verify-enrollment", map[string]string{
		"code": code,
	}, tok).ok(t, nil)

	var lr e2eLoginResult
	e.post("/api/v1/auth/login", map[string]string{
		"account":  account,
		"password": "password123",
	}, "").code(t, 200, &lr)
	if !lr.Need2FA || lr.ChallengeToken == "" {
		t.Fatalf("need_2fa must be true with challenge_token, got %v", lr.Need2FA)
	}

	code2, err := totp.GenerateCode(enr.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code 2: %v", err)
	}
	var verifyRes struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	e.post("/api/v1/auth/2fa/verify", map[string]string{
		"challenge_token": lr.ChallengeToken,
		"code":            code2,
	}, "").ok(t, &verifyRes)
	if verifyRes.Tokens.AccessToken == "" {
		t.Fatal("2FA verify must return real access token")
	}

	var me struct {
		ID string `json:"id"`
	}
	e.get("/api/v1/users/me", verifyRes.Tokens.AccessToken).ok(t, &me)

	var wrongLR e2eLoginResult
	e.post("/api/v1/auth/login", map[string]string{
		"account":  account,
		"password": "password123",
	}, "").code(t, 200, &wrongLR)
	resp := e.post("/api/v1/auth/2fa/verify", map[string]string{
		"challenge_token": wrongLR.ChallengeToken,
		"code":            "000000",
	}, "")
	if resp.status != 401 {
		t.Fatalf("wrong 2fa code must return 401, got %d", resp.status)
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: Password reset anti-enumeration + error paths
// ---------------------------------------------------------------------------

func TestE2E_PasswordResetFlow(t *testing.T) {
	e := newE2E(t)

	resp := e.post("/api/v1/auth/password-reset/request", map[string]string{
		"account": "nobody@e2e.test",
	}, "")
	if resp.status != 200 {
		t.Fatalf("password-reset/request must return 200 for unknown account, got %d", resp.status)
	}

	resp2 := e.post("/api/v1/auth/password-reset/complete", map[string]string{
		"token":        "bad-token",
		"new_password": "newpassword123",
	}, "")
	if resp2.status != 401 {
		t.Fatalf("bad token must return 401, got %d", resp2.status)
	}

	resp3 := e.post("/api/v1/auth/password-reset/complete", map[string]string{
		"token":        "some-token",
		"new_password": "short",
	}, "")
	if resp3.status != 400 {
		t.Fatalf("short password must return 400, got %d", resp3.status)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: Withdrawal approval flow (cross-module: auth → withdrawal)
// ---------------------------------------------------------------------------

func TestE2E_WithdrawalApprovalFlow(t *testing.T) {
	e := newE2E(t)
	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("wseller"), "password123")
	_, opsID := e.registerAndLogin(uniqueAccount("wops"), "password123")
	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)

	// Seed a dataset + version so the settled order FK chain is satisfied.
	e.seedQuery(t, `
		INSERT INTO datasets (id, seller_id, title, description, data_type, license_type, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'WD Test DS', 'Seeded', 'text', 'commercial', 'published', now(), now())
	`, sellerID)
	var dsID string
	e.seedQueryRow(t, []any{&dsID},
		`SELECT id FROM datasets WHERE seller_id=$1 AND title='WD Test DS' LIMIT 1`, sellerID)
	e.seedQuery(t, `
		INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
		VALUES (gen_random_uuid(), $1, 1, '[]', now())
	`, dsID)
	var verID string
	e.seedQueryRow(t, []any{&verID},
		`SELECT id FROM dataset_versions WHERE dataset_id=$1 LIMIT 1`, dsID)

	// Seed a settled order so the seller has withdrawable balance.
	e.seedQuery(t, `
		INSERT INTO orders (id, buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 'commercial',
			5000, 500, 4500, 'settled', 'download', now(), now())
	`, sellerID, sellerID, dsID, verID)

	// Seller files withdrawal.
	type wd struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	var w wd
	e.post("/api/v1/sellers/me/withdrawals", map[string]any{
		"amount_cents":  3000,
		"channel":       "bank",
		"account_label": "test-bank-1234",
	}, sellerTok).ok(t, &w)
	if w.Status != "pending" {
		t.Fatalf("withdrawal status after request = %q, want pending", w.Status)
	}

	// Promote ops user. Role change invalidates the old token — reissue by login.
	e.seedQuery(t, `UPDATE users SET role='ops' WHERE id=$1`, opsID)
	var opsAccount string
	e.seedQueryRow(t, []any{&opsAccount}, `SELECT account FROM users WHERE id=$1`, opsID)
	var loginRes struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	e.post("/api/v1/auth/login", map[string]any{
		"account":  opsAccount,
		"password": "password123",
	}, "").ok(t, &loginRes)
	opsTok := loginRes.Tokens.AccessToken

	// Approve → complete.
	e.post("/api/v1/admin/withdrawals/"+w.ID+"/approve", map[string]any{"note": "e2e"}, opsTok).ok(t, &w)
	if w.Status != "approved" {
		t.Fatalf("withdrawal after approve = %q, want approved", w.Status)
	}
	e.post("/api/v1/admin/withdrawals/"+w.ID+"/complete", map[string]any{"note": "e2e"}, opsTok).ok(t, &w)
	if w.Status != "completed" {
		t.Fatalf("withdrawal after complete = %q, want completed", w.Status)
	}

	// Seller should now have a withdrawal_completed notification.
	var notifs struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	e.get("/api/v1/users/me/notifications", sellerTok).ok(t, &notifs)
	found := false
	for _, n := range notifs.Items {
		if n.Kind == "withdrawal_completed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected withdrawal_completed notification, got %+v", notifs.Items)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: Migration roundtrip (down → up with embedded golang-migrate Go API)
// ---------------------------------------------------------------------------

func TestE2E_MigrationRoundtrip(t *testing.T) {
	// Verify every migration's down step is reversible by running
	// Up → Down → Up against a freshly created database, so production
	// `migrate down`-style rollbacks don't blow up in an incident.
	//
	// We must not touch the shared E2E database (other tests have rows in
	// it).  Strategy: parse DATABASE_URL, connect to the postgres maintenance
	// database, CREATE DATABASE <unique>, run the cycle there, DROP DATABASE.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping migration roundtrip")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}

	// Maintenance DSN points at the standard "postgres" database so we can
	// CREATE/DROP the target without holding a connection on it.
	mu := *u
	mu.Path = "/postgres"
	mctl, err := sql.Open("pgx", mu.String())
	if err != nil {
		t.Fatalf("open maintenance: %v", err)
	}
	defer mctl.Close()
	if err := mctl.Ping(); err != nil {
		t.Fatalf("ping maintenance: %v", err)
	}

	// Build a unique database name (PG identifiers max 63 bytes; we use < 30).
	dbName := fmt.Sprintf("e2e_mig_%d", time.Now().UnixNano()%1_000_000_000)
	if _, err := mctl.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		// Kick any lingering backends so DROP DATABASE succeeds even if a
		// migrate driver lagged on its FD close.
		_, _ = mctl.Exec(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1",
			dbName)
		if _, err := mctl.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			t.Logf("drop %s: %v", dbName, err)
		}
	})

	// Roundtrip DSN: same auth/host, but target the temp database.
	ru := *u
	ru.Path = "/" + dbName
	tempDSN := ru.String()

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}

	// Helper: build a fresh migrator (golang-migrate's instance is tied to its
	// own DB connection — close + reopen between Up/Down/Up keeps the API simple).
	newMigrator := func() *migrate.Migrate {
		t.Helper()
		// pgxScheme rewrites postgres:// → pgx5:// to match the registered driver.
		mig, err := migrate.NewWithSourceInstance("iofs", src, pgxMigrateScheme(tempDSN))
		if err != nil {
			t.Fatalf("init migrator: %v", err)
		}
		return mig
	}

	// 1. Up (all migrations apply on a fresh DB).
	m1 := newMigrator()
	if err := m1.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("first Up: %v", err)
	}
	_, _ = m1.Close()

	// 2. Down (every migration has a working down step).
	m2 := newMigrator()
	if err := m2.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("Down: %v", err)
	}
	_, _ = m2.Close()

	// 3. Up again (migrations re-apply on the now-empty schema).
	m3 := newMigrator()
	if err := m3.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("second Up: %v", err)
	}
	_, _ = m3.Close()
}

// pgxMigrateScheme mirrors backend/internal/platform/db/migrate.go (unexported there).
// We duplicate one line rather than open the platform/db package up.
func pgxMigrateScheme(dsn string) string {
	const old = "postgres://"
	const newp = "pgx5://"
	if strings.HasPrefix(dsn, old) {
		return newp + dsn[len(old):]
	}
	return dsn
}
