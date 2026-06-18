package auth

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"golang.org/x/crypto/bcrypt"
)

func TestEnroll2FA_GeneratesSecretAndRecoveryCodes(t *testing.T) {
	svc, repo := newTestService()
	repo.byID["u1"] = User{ID: "u1", Account: "a@b.com"}

	res, err := svc.Enroll2FA(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RecoveryCodes) != 8 {
		t.Fatalf("recovery codes = %d, want 8", len(res.RecoveryCodes))
	}
	if res.Secret == "" || res.OTPAuthURL == "" {
		t.Fatal("secret and otpauth_url must not be empty")
	}
}

// Re-enrolling before verifying (UI back/retry) overwrites the TOTP secret but
// must NOT leave the previous attempt's recovery codes valid — otherwise codes
// shown in an abandoned attempt (possibly screenshotted/logged) stay live and
// decoupled from the current secret. Only the latest 8 codes may remain.
func TestEnroll2FA_ReEnrollClearsStaleRecoveryCodes(t *testing.T) {
	svc, repo := newTestService()
	repo.byID["u1"] = User{ID: "u1", Account: "a@b.com"}

	if _, err := svc.Enroll2FA(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enroll2FA(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}

	if n := len(repo.recoveryCodes["u1"]); n != 8 {
		t.Fatalf("stored recovery codes after re-enroll = %d, want 8 (stale codes must be cleared)", n)
	}
}

func TestEnroll2FA_RejectsWhenAlreadyEnabled(t *testing.T) {
	svc, repo := newTestService()
	repo.byID["u1"] = User{ID: "u1", TOTPEnabled: true}

	_, err := svc.Enroll2FA(context.Background(), "u1")
	if err != ErrAlready2FAEnabled {
		t.Fatalf("want ErrAlready2FAEnabled, got %v", err)
	}
}

func TestLogin_With2FAEnabled_ReturnsChallenge(t *testing.T) {
	svc, repo := newTestService()
	repo.byAccount["a@x.com"] = userWithHash{
		user: User{ID: "u-2fa", Account: "a@x.com"}, hash: hashPassword("pw"),
	}
	repo.byID["u-2fa"] = User{ID: "u-2fa", Account: "a@x.com", Role: "buyer", TOTPEnabled: true}

	res, err := svc.Login(context.Background(), "a@x.com", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Need2FA || res.ChallengeToken == "" {
		t.Fatalf("Need2FA=%v challenge=%q", res.Need2FA, res.ChallengeToken)
	}
	if res.Tokens != nil {
		t.Fatal("tokens must be nil when 2FA enabled")
	}
}

func TestRequestPasswordReset_DoesNotRevealUserExistence(t *testing.T) {
	_, _ = newTestService()
	svc := &Service{
		repo:       &fakeRepo{byAccount: map[string]userWithHash{}},
		appBaseURL: "https://app",
	}
	// Non-existent account must not error.
	if err := svc.RequestPasswordReset(context.Background(), "nobody@x.com"); err != nil {
		t.Fatalf("must not reveal existence: got %v", err)
	}
}

func TestCompletePasswordReset_RejectsExpiredToken(t *testing.T) {
	rawToken := "some-raw-token"
	tokenHash := sha256Hex(rawToken)
	svc := &Service{
		repo: &fakeRepo{
			resetTokens: map[string]passwordResetTokenRow{
				tokenHash: {TokenHash: tokenHash, UserID: "u1", ExpiresAt: time.Now().Add(-time.Hour)},
			},
		},
	}
	err := svc.CompletePasswordReset(context.Background(), rawToken, "newpassword123")
	if err != ErrTokenInvalidOrExpired {
		t.Fatalf("want ErrTokenInvalidOrExpired, got %v", err)
	}
}

func hashPassword(pw string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	return string(h)
}

// Test2FALoginFlow is a full integration test of the 2FA TOTP flow:
// 1. Register user → enroll 2FA → verify enrollment → totp_enabled=true
// 2. Login → need_2fa + challenge_token (no real tokens)
// TestVerify2FAChallenge_RejectsFrozenUser: a frozen/banned account must not
// obtain tokens via the 2FA completion step (Login/Refresh already reject it).
func TestVerify2FAChallenge_RejectsFrozenUser(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	res, err := svc.Register(ctx, "frozen2fa@test.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := res.User.ID
	repo.byID[userID] = res.User
	enroll, _ := svc.Enroll2FA(ctx, userID)
	code, _ := totp.GenerateCode(enroll.Secret, time.Now())
	if err := svc.Verify2FAEnrollment(ctx, userID, code); err != nil {
		t.Fatalf("verify enrollment: %v", err)
	}
	loginRes, _ := svc.Login(ctx, "frozen2fa@test.com", "password123")

	// Freeze the account after the challenge was issued.
	u := repo.byID[userID]
	u.Status = statusFrozen
	repo.byID[userID] = u

	code2, _ := totp.GenerateCode(enroll.Secret, time.Now())
	if _, _, err := svc.Verify2FAChallenge(ctx, loginRes.ChallengeToken, code2); err != ErrUserFrozen {
		t.Fatalf("frozen user via 2FA = %v, want ErrUserFrozen", err)
	}
}

// 3. Verify2FAChallenge with valid TOTP code → real access+refresh tokens
// 4. Wrong password → ErrInvalidCredentials
// 5. Wrong TOTP code → ErrInvalid2FACode
// 6. RecoveryCodeStatus returns correct unused count
func Test2FALoginFlow(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	// Register a user.
	res, err := svc.Register(ctx, "2fa@test.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := res.User.ID
	repo.byID[userID] = res.User

	// 1. Enroll 2FA.
	enrollRes, err := svc.Enroll2FA(ctx, userID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if len(enrollRes.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(enrollRes.RecoveryCodes))
	}
	if enrollRes.Secret == "" {
		t.Fatal("secret must not be empty")
	}

	// 2. Verify enrollment with a real TOTP code.
	code, err := totp.GenerateCode(enrollRes.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	if err := svc.Verify2FAEnrollment(ctx, userID, code); err != nil {
		t.Fatalf("verify enrollment: %v", err)
	}

	// 3. Login → need_2fa.
	loginRes, err := svc.Login(ctx, "2fa@test.com", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !loginRes.Need2FA || loginRes.ChallengeToken == "" {
		t.Fatalf("expected need_2fa=true with challenge, got need_2fa=%v token=%q",
			loginRes.Need2FA, loginRes.ChallengeToken)
	}
	if loginRes.Tokens != nil {
		t.Fatal("tokens must be nil when 2FA is enabled")
	}

	// 4. Verify 2FA challenge → real tokens issued.
	code2, err := totp.GenerateCode(enrollRes.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	tokens, u, err := svc.Verify2FAChallenge(ctx, loginRes.ChallengeToken, code2)
	if err != nil {
		t.Fatalf("verify 2fa challenge: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected real access+refresh tokens after 2FA")
	}
	if u.ID != userID {
		t.Fatalf("expected uid=%s, got %s", userID, u.ID)
	}

	// Tokens are valid JWT with correct type.
	claims, err := svc.tokens.Parse(tokens.AccessToken, tokenTypeAccess)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("token uid=%s, want %s", claims.UserID, userID)
	}

	// 5. Wrong password → rejected.
	_, err = svc.Login(ctx, "2fa@test.com", "wrong")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got %v", err)
	}

	// 6. Wrong TOTP code → rejected.
	loginRes2, _ := svc.Login(ctx, "2fa@test.com", "password123")
	_, _, err = svc.Verify2FAChallenge(ctx, loginRes2.ChallengeToken, "000000")
	if err != ErrInvalid2FACode {
		t.Fatalf("expected ErrInvalid2FACode for wrong code, got %v", err)
	}

	// 7. Recovery code status.
	n, err := svc.RecoveryCodeStatus(ctx, userID)
	if err != nil {
		t.Fatalf("recovery code status: %v", err)
	}
	if n != 8 {
		t.Fatalf("expected 8 unused recovery codes, got %d", n)
	}
}
