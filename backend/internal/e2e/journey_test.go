package e2e

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// ---------------------------------------------------------------------------
// Scenario 1: 2FA login full flow (PR-V end-to-end verification)
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
// Scenario 2: Password reset anti-enumeration + error paths
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
