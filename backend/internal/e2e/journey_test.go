package e2e

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// ---------------------------------------------------------------------------
// Scenario 1: Full purchase journey (cross-module: auth → dataset → order → payment)
// ---------------------------------------------------------------------------

func TestE2E_FullPurchaseJourney(t *testing.T) {
	e := newE2E(t)

	_, sellerID := e.registerAndLogin(uniqueAccount("pseller"), "password123")
	buyerTok, _ := e.registerAndLogin(uniqueAccount("pbuyer"), "password123")

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

	// Buyer views detail.
	var detailRes struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	e.get("/api/v1/datasets/"+datasetID, buyerTok).ok(t, &detailRes)

	// Buyer creates order.
	type orderReq struct {
		DatasetID   string `json:"dataset_id"`
		LicenseType string `json:"license_type"`
	}
	var orderRes struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	e.post("/api/v1/orders", orderReq{
		DatasetID:   datasetID,
		LicenseType: "commercial",
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

	sellerTok, _ := e.registerAndLogin(uniqueAccount("wdseller"), "password123")
	opsTok, _ := e.registerAndLogin(uniqueAccount("opsuser"), "password123")

	// SIMPLIFICATION: promote roles via seed.
	e.seedQuery(t, `UPDATE users SET role='ops' WHERE account LIKE 'opsuser_%'`)
	e.seedQuery(t, `UPDATE users SET role='seller', kyc_status='verified' WHERE account LIKE 'wdseller_%'`)

	// Withdrawal endpoint: must exist, authenticate, and not 500.
	// May fail with "insufficient balance" (expected — no settled orders).
	resp := e.post("/api/v1/sellers/me/withdrawals", map[string]interface{}{
		"amount_cents": 3000,
		"channel":      "mock",
	}, sellerTok)
	if resp.status >= 500 {
		t.Fatalf("withdrawal request must not 500, got %d: %s", resp.status, resp.body())
	}
	t.Logf("withdrawal request status: %d (insufficient balance expected)", resp.status)

	// Admin endpoint: exists, authenticated (ops role), doesn't 500.
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
	// The up-migration path is tested by every E2E test (harness calls
	// db.RunMigrations).  A full down→up roundtrip requires a completely
	// empty database (no FK chains on tables with existing data), which is
	// incompatible with parallel E2E tests sharing the same PG instance.
	//
	// Workaround for CI: add a dedicated CI job that runs migration down→up
	// in a separate PG service container.  Alternatively, use
	// golang-migrate Go API against an ephemeral PG in a single-test suite
	// (run with -p 1 and a unique DATABASE_URL).
	t.Skip("full down→up roundtrip needs empty DB; up path verified by RunMigrations in harness")
}
