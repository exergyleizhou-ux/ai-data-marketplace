package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/pquerna/otp/totp"

	"github.com/lei/ai-data-marketplace/backend/migrations"
)

// ---------------------------------------------------------------------------
// Scenario 1: Full purchase journey (cross-module: auth → dataset → order → payment)
// ---------------------------------------------------------------------------

func TestE2E_FullPurchaseJourney(t *testing.T) {
	e := newE2E(t)

	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("pseller"), "password123")
	buyerTok, _ := e.registerAndLogin(uniqueAccount("pbuyer"), "password123")

	// KYC auto-approve is on in e2e config, but applies on KYC submission,
	// not registration.  Seed verified directly.
	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)

	// Create dataset through the API — the real HTTP contract test.
	type createDS struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DataType    string `json:"data_type"`
		LicenseType string `json:"license_type"`
	}
	var dsRes struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	e.post("/api/v1/datasets", createDS{
		Title:       "E2E Purchase Dataset",
		Description: "Seeded dataset for cross-module E2E purchase journey",
		DataType:    "text",
		LicenseType: "commercial",
	}, sellerTok).ok(t, &dsRes)
	if dsRes.ID == "" {
		t.Fatal("dataset id must not be empty")
	}
	datasetID := dsRes.ID

	// SIMPLIFICATION: skip upload/review by seeding dataset to published state
	// and creating a version row for the orders FK.  This lets us focus on
	// the order→payment→delivery cross-module contract.
	verID := fmt.Sprintf("e2e-ver-%d", time.Now().UnixNano())
	e.seedQuery(t, `
		INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
		VALUES ($1, $2, 1, '[]', now())
	`, verID, datasetID)
	e.seedQuery(t, `
		UPDATE datasets SET status='published', current_version_id=$1, updated_at=now()
		WHERE id=$2
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

	// Buyer views detail.
	var detailRes struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	e.get("/api/v1/datasets/"+datasetID, buyerTok).ok(t, &detailRes)

	// Buyer creates order.
	type orderReq struct {
		DatasetID string `json:"dataset_id"`
		VersionID string `json:"version_id"`
	}
	var orderRes struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	e.post("/api/v1/orders", orderReq{
		DatasetID: datasetID,
		VersionID: verID,
	}, buyerTok).ok(t, &orderRes)
	if orderRes.ID == "" {
		t.Fatal("order id must not be empty")
	}
	orderID := orderRes.ID

	// Buyer pays (mock provider).
	type payReq struct {
		OrderID string `json:"order_id"`
		Channel string `json:"channel"`
	}
	var payRes struct {
		Status string `json:"status"`
	}
	e.post("/api/v1/payments", payReq{
		OrderID: orderID,
		Channel: "mock",
	}, buyerTok).ok(t, &payRes)
	if payRes.Status != "paid" && payRes.Status != "confirmed" {
		t.Fatalf("payment status must be paid/confirmed, got %s", payRes.Status)
	}

	// Verify order reached a terminal state.
	var orderAfter struct {
		Status string `json:"status"`
	}
	e.get("/api/v1/orders/"+orderID, buyerTok).ok(t, &orderAfter)
	valid := map[string]bool{"paid": true, "settled": true, "delivered": true, "confirmed": true}
	if !valid[orderAfter.Status] {
		t.Errorf("order in unexpected final state: %s", orderAfter.Status)
	}
	t.Logf("full purchase journey: dataset=%s order=%s final=%s", datasetID, orderID, orderAfter.Status)
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

	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("wdseller"), "password123")
	opsTok, _ := e.registerAndLogin(uniqueAccount("opsuser"), "password123")

	// SIMPLIFICATION: promote roles via seed.
	e.seedQuery(t, `UPDATE users SET role='ops' WHERE account LIKE 'opsuser_%'`)
	e.seedQuery(t, `UPDATE users SET role='seller', kyc_status='verified' WHERE id=$1`, sellerID)

	// SIMPLIFICATION: seed a settled order for seller balance.
	verID := fmt.Sprintf("e2e-wd-ver-%d", time.Now().UnixNano())
	e.seedQuery(t, `
		INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
		VALUES ($1, (SELECT id FROM datasets WHERE seller_id=$2 LIMIT 1), 1, '[]', now())
	`, verID, sellerID)
	e.seedQuery(t, `
		INSERT INTO orders (id, buyer_id, seller_id, dataset_id, version_id,
			license_type, amount_cents, platform_fee_cents, seller_amount_cents,
			status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $1,
			(SELECT id FROM datasets WHERE seller_id=$1 LIMIT 1), $2,
			'commercial', 5000, 500, 4500, 'settled', now(), now())
	`, sellerID, verID)

	// Seller requests withdrawal — must not 500.
	resp := e.post("/api/v1/sellers/me/withdrawals", map[string]interface{}{
		"amount_cents": 3000,
		"channel":      "mock",
	}, sellerTok)
	if resp.status >= 500 {
		t.Fatalf("withdrawal request must not 500, got %d: %s", resp.status, resp.body())
	}
	t.Logf("withdrawal request status: %d", resp.status)

	// Admin endpoint exists and doesn't 500.
	resp2 := e.get("/api/v1/admin/withdrawals?status=pending", opsTok)
	if resp2.status >= 500 {
		t.Fatalf("admin withdrawals must not 500, got %d", resp2.status)
	}
	t.Logf("admin withdrawals (ops) status: %d", resp2.status)
}

// ---------------------------------------------------------------------------
// Scenario 5: Migration roundtrip (down → up with embedded golang-migrate Go API)
// ---------------------------------------------------------------------------

func TestE2E_MigrationRoundtrip(t *testing.T) {
	e := newE2E(t)

	dsn := e.dbDSN
	// Rewrite to pgx5:// scheme.
	scheme := dsn
	for _, p := range []string{"postgres://", "postgresql://"} {
		if len(scheme) >= len(p) && scheme[:len(p)] == p {
			scheme = "pgx5://" + scheme[len(p):]
			break
		}
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, scheme)
	if err != nil {
		t.Fatalf("init migrator: %v", err)
	}

	// Down all, then up all (the real roundtrip test).
	if err := m.Down(); err != nil {
		t.Fatalf("down all: %v", err)
	}
	sourceErr, dbErr := m.Close()
	_ = sourceErr
	_ = dbErr

	m2, err := migrate.NewWithSourceInstance("iofs", src, scheme)
	if err != nil {
		t.Fatalf("reinit after down: %v", err)
	}
	if err := m2.Up(); err != nil {
		t.Fatalf("re-up after down: %v", err)
	}
	sourceErr2, dbErr2 := m2.Close()
	_ = sourceErr2
	_ = dbErr2

	// Count total migrations for the log.
	entries, _ := migrations.FS.ReadDir(".")
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 7 && entry.Name()[len(entry.Name())-7:] == ".up.sql" {
			count++
		}
	}
	t.Logf("migration roundtrip OK: %d migrations down→up", count)
}

var _ = context.Background
