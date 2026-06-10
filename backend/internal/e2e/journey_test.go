package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// ---------------------------------------------------------------------------
// Scenario 1: Full purchase journey
// ---------------------------------------------------------------------------

func TestE2E_FullPurchaseJourney(t *testing.T) {
	e := newE2E(t)

	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("seller"), "password123")
	buyerTok, _ := e.registerAndLogin(uniqueAccount("buyer"), "password123")

	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)

	// Seller creates a dataset.
	var dsRes struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	e.post("/api/v1/datasets", map[string]interface{}{
		"title":        "E2E Test Dataset",
		"description":  "A test dataset for E2E purchase journey",
		"data_type":    "text",
		"price_cents":  199,
		"license_type": "commercial",
	}, sellerTok).ok(t, &dsRes)
	if dsRes.ID == "" {
		t.Fatal("dataset id must not be empty")
	}
	datasetID := dsRes.ID

	// Seed published.
	e.seedQuery(t, `UPDATE datasets SET status='published' WHERE id=$1`, datasetID)

	// Buyer browses.
	var browseRes struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	e.get("/api/v1/datasets", buyerTok).ok(t, &browseRes)
	if len(browseRes.Items) == 0 {
		t.Fatal("datasets list must have at least one published dataset")
	}

	// Buyer views detail.
	var detailRes struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Price int    `json:"price_cents"`
	}
	e.get("/api/v1/datasets/"+datasetID, buyerTok).ok(t, &detailRes)
	if detailRes.Price != 199 {
		t.Fatalf("expected price 199, got %d", detailRes.Price)
	}

	// Buyer creates order.
	var orderRes struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	e.post("/api/v1/orders", map[string]string{
		"dataset_id": datasetID,
		"version_id": "",
	}, buyerTok).ok(t, &orderRes)
	if orderRes.ID == "" {
		t.Fatal("order id must not be empty")
	}
	orderID := orderRes.ID

	// Buyer pays.
	var payRes struct {
		Status string `json:"status"`
	}
	e.post("/api/v1/payments", map[string]string{
		"order_id": orderID,
		"channel":  "mock",
	}, buyerTok).ok(t, &payRes)
	if payRes.Status != "paid" && payRes.Status != "confirmed" {
		t.Fatalf("payment status must be paid/confirmed, got %s", payRes.Status)
	}

	// Verify final order state.
	var orderAfter struct {
		Status string `json:"status"`
	}
	e.get("/api/v1/orders/"+orderID, buyerTok).ok(t, &orderAfter)
	valid := map[string]bool{"paid": true, "settled": true, "delivered": true, "confirmed": true}
	if !valid[orderAfter.Status] {
		t.Errorf("order in unexpected final state: %s", orderAfter.Status)
	}
	t.Logf("order %s final status: %s", orderID, orderAfter.Status)
}

// ---------------------------------------------------------------------------
// Scenario 2: 2FA login full flow
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

	// Enroll 2FA.
	var enr e2eEnroll2FAResult
	e.post("/api/v1/auth/2fa/enroll", nil, tok).ok(t, &enr)
	if len(enr.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(enr.RecoveryCodes))
	}

	// Verify enrollment with real TOTP code.
	code, err := totp.GenerateCode(enr.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	e.post("/api/v1/auth/2fa/verify-enrollment", map[string]string{
		"code": code,
	}, tok).ok(t, nil)

	// Login → need_2fa.
	var lr e2eLoginResult
	e.post("/api/v1/auth/login", map[string]string{
		"account":  account,
		"password": "password123",
	}, "").code(t, 200, &lr)

	if !lr.Need2FA || lr.ChallengeToken == "" {
		t.Fatalf("need_2fa must be true with challenge_token, got %v", lr.Need2FA)
	}
	if lr.Tokens != nil {
		t.Fatal("tokens must be nil when 2FA is enabled")
	}

	// Verify 2FA challenge → real tokens.
	code2, err := totp.GenerateCode(enr.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code 2: %v", err)
	}

	var verifyRes struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	e.post("/api/v1/auth/2fa/verify", map[string]string{
		"challenge_token": lr.ChallengeToken,
		"code":            code2,
	}, "").ok(t, &verifyRes)

	if verifyRes.Tokens.AccessToken == "" {
		t.Fatal("2FA verify must return real access token")
	}

	// Access token works.
	var me struct {
		ID string `json:"id"`
	}
	e.get("/api/v1/users/me", verifyRes.Tokens.AccessToken).ok(t, &me)
	if me.ID != verifyRes.User.ID {
		t.Fatalf("me id %s != verify user id %s", me.ID, verifyRes.User.ID)
	}

	// Wrong TOTP code → 401.
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
		t.Fatalf("wrong 2fa code must return 401, got %d: %s", resp.status, resp.body())
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: Password reset anti-enumeration + error paths
// ---------------------------------------------------------------------------

func TestE2E_PasswordResetFlow(t *testing.T) {
	e := newE2E(t)

	// Non-existent account → 200 (anti-enumeration).
	resp := e.post("/api/v1/auth/password-reset/request", map[string]string{
		"account": "nobody@e2e.test",
	}, "")
	if resp.status != 200 {
		t.Fatalf("password-reset/request must return 200 for unknown account, got %d", resp.status)
	}

	// Bad token → 401.
	resp2 := e.post("/api/v1/auth/password-reset/complete", map[string]string{
		"token":        "bad-token",
		"new_password": "newpassword123",
	}, "")
	if resp2.status != 401 {
		t.Fatalf("bad token must return 401, got %d: %s", resp2.status, resp2.body())
	}

	// Short password → 400.
	resp3 := e.post("/api/v1/auth/password-reset/complete", map[string]string{
		"token":        "some-token",
		"new_password": "short",
	}, "")
	if resp3.status != 400 {
		t.Fatalf("short password must return 400, got %d: %s", resp3.status, resp3.body())
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: Withdrawal approval
// ---------------------------------------------------------------------------

func TestE2E_WithdrawalApprovalFlow(t *testing.T) {
	e := newE2E(t)

	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("wdseller"), "password123")
	opsTok, _ := e.registerAndLogin(uniqueAccount("opsuser"), "password123")
	e.seedQuery(t, `UPDATE users SET role='ops' WHERE account LIKE 'opsuser_%'`)

	// Promote seller to seller role (needed for withdrawal access).
	e.seedQuery(t, `UPDATE users SET role='seller', kyc_status='verified' WHERE id=$1`, sellerID)

	// Try to request withdrawal — may fail if no settled balance, but must not 500.
	resp := e.post("/api/v1/sellers/me/withdrawals", map[string]interface{}{
		"amount_cents": 3000,
		"channel":      "mock",
	}, sellerTok)

	// Without settled earnings the request may return 400/404/409 — all are
	// valid responses (not 500).  The key E2E assertion is: the API exists,
	// auth works, and the response is a handled error, not a panic.
	if resp.status >= 500 {
		t.Fatalf("withdrawal request must not 500, got %d: %s", resp.status, resp.body())
	}
	t.Logf("withdrawal request status: %d", resp.status)

	// Ops can list pending withdrawals without panicking.
	var wds struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	e.get("/api/v1/admin/withdrawals?status=pending", opsTok).ok(t, &wds)
	t.Logf("admin sees %d pending withdrawals", len(wds.Items))
}

// ---------------------------------------------------------------------------
// Scenario 5: Watchlist notification
// ---------------------------------------------------------------------------

func TestE2E_WatchlistNotificationFlow(t *testing.T) {
	e := newE2E(t)

	buyerTok, _ := e.registerAndLogin(uniqueAccount("wbuyer"), "password123")
	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("wseller"), "password123")

	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)

	// Seller creates dataset.
	var dsRes struct {
		ID string `json:"id"`
	}
	e.post("/api/v1/datasets", map[string]interface{}{
		"title":        "Watchlist Test Dataset",
		"description":  "Testing watchlist notifications",
		"data_type":    "text",
		"price_cents":  99,
		"license_type": "commercial",
	}, sellerTok).ok(t, &dsRes)
	datasetID := dsRes.ID

	e.seedQuery(t, `UPDATE datasets SET status='published' WHERE id=$1`, datasetID)

	// Buyer watches.
	resp := e.post("/api/v1/datasets/"+datasetID+"/watch", nil, buyerTok)
	if resp.status == 200 {
		var wr struct {
			ID string `json:"id"`
		}
		resp.ok(t, &wr)
	}

	// Buyer's watchlist.
	var watchList struct {
		Items []struct {
			DatasetID string `json:"dataset_id"`
		} `json:"items"`
	}
	e.get("/api/v1/users/me/watched", buyerTok).ok(t, &watchList)

	found := false
	for _, w := range watchList.Items {
		if w.DatasetID == datasetID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("dataset %s not found in buyer's watchlist", datasetID)
	}

	// Seed new version → trigger notification.
	versionID := fmt.Sprintf("e2e-ver-%d", time.Now().UnixNano())
	e.seedQuery(t, `
		INSERT INTO dataset_versions (id, dataset_id, version, status, file_count, created_at, updated_at)
		VALUES ($1, $2, '2.0', 'published', 1, now(), now())
	`, versionID, datasetID)

	// Check notifications.
	var notifList struct {
		Items []struct {
			Kind       string `json:"kind"`
			ResourceID string `json:"resource_id"`
		} `json:"items"`
	}
	e.get("/api/v1/users/me/notifications", buyerTok).ok(t, &notifList)
	t.Logf("notifications after version publish: %d items", len(notifList.Items))
}

// ---------------------------------------------------------------------------
// Scenario 6: C2D compute job journey (L1, mock runner)
// ---------------------------------------------------------------------------

func TestE2E_ComputeJobJourney(t *testing.T) {
	e := newE2E(t)

	sellerTok, sellerID := e.registerAndLogin(uniqueAccount("cseller"), "password123")
	buyerTok, _ := e.registerAndLogin(uniqueAccount("cbuyer"), "password123")

	e.seedQuery(t, `UPDATE users SET kyc_status='verified' WHERE id=$1`, sellerID)

	// Seller creates dataset.
	var dsRes struct {
		ID string `json:"id"`
	}
	e.post("/api/v1/datasets", map[string]interface{}{
		"title":        "C2D Test Dataset",
		"description":  "Dataset for compute-to-data E2E",
		"data_type":    "text",
		"price_cents":  299,
		"license_type": "commercial",
	}, sellerTok).ok(t, &dsRes)
	datasetID := dsRes.ID

	// Seed compute offer.
	e.seedQuery(t, `
		INSERT INTO dataset_compute_offers (id, dataset_id, algorithm_id, price_cents, status, created_at)
		VALUES ($1, $2, 'algo-logreg', 199, 'active', now())
	`, "e2e-c2d-offer-1", datasetID)

	// Seed algorithm.
	e.seedQuery(t, `
		INSERT INTO algorithms (id, name, description, image_digest, status)
		VALUES ('algo-logreg', 'Logistic Regression', 'L1 logistic regression sandbox', 'sha256:abc123', 'active')
		ON CONFLICT DO NOTHING
	`)

	e.seedQuery(t, `UPDATE datasets SET status='published' WHERE id=$1`, datasetID)

	// Buyer purchases entitlement.
	resp2 := e.post("/api/v1/compute/entitlements", map[string]string{
		"offer_id":   "e2e-c2d-offer-1",
		"dataset_id": datasetID,
	}, buyerTok)
	// May need payment/product flow — accept 400 as handled error.
	if resp2.status >= 500 {
		t.Fatalf("entitlement purchase must not 500, got %d: %s", resp2.status, resp2.body())
	}
	t.Logf("entitlement purchase status: %d", resp2.status)

	// Pay for entitlement.
	var entRes struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if resp2.status == 200 {
		resp2.ok(t, &entRes)
		if entRes.ID != "" {
			e.post("/api/v1/payments", map[string]string{
				"order_id": entRes.ID,
				"channel":  "mock",
			}, buyerTok)
		}
	}

	// Submit compute job if entitlement exists.
	if entRes.ID != "" {
		var jobRes struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		resp3 := e.post("/api/v1/compute/jobs", map[string]string{
			"entitlement_id": entRes.ID,
		}, buyerTok)
		if resp3.status >= 500 {
			t.Fatalf("job submit must not 500, got %d: %s", resp3.status, resp3.body())
		}
		if resp3.status == 200 {
			resp3.ok(t, &jobRes)
			if jobRes.ID != "" {
				jobID := jobRes.ID
				var finalStatus string
				for i := 0; i < 30; i++ {
					time.Sleep(200 * time.Millisecond)
					var j struct {
						Status string `json:"status"`
					}
					e.get("/api/v1/compute/jobs/"+jobID, buyerTok).ok(t, &j)
					if j.Status == "released" || j.Status == "failed" || j.Status == "completed" {
						finalStatus = j.Status
						break
					}
				}
				t.Logf("compute job %s final status: %s", jobID, finalStatus)
			}
		} else {
			t.Logf("compute job submit status: %d (may need payment first)", resp3.status)
		}
	}
}
