package auth

import (
	"context"
	"testing"
	"time"

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
