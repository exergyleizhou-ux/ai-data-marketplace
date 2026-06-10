package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func lockoutSvc(t *testing.T) (*Service, *fakeRepo, string, string) {
	t.Helper()
	svc, repo := newTestService()
	const account, password = "lock@test.dev", "correct-horse-9"
	res, err := svc.Register(context.Background(), account, "email", password)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, repo, account, res.User.ID
}

// 5 consecutive failures lock the account: even the CORRECT password is then
// rejected — with the same generic error (no enumeration signal).
func TestLogin_LocksAfterMaxFailures(t *testing.T) {
	svc, _, account, _ := lockoutSvc(t)
	ctx := context.Background()
	for i := 0; i < maxLoginFailures; i++ {
		if _, err := svc.Login(ctx, account, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: want ErrInvalidCredentials, got %v", i+1, err)
		}
	}
	if _, err := svc.Login(ctx, account, "correct-horse-9"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked account must reject the correct password with the SAME generic error, got %v", err)
	}
}

// A successful login resets the counter — failures don't accumulate forever.
func TestLogin_SuccessResetsFailureCounter(t *testing.T) {
	svc, _, account, _ := lockoutSvc(t)
	ctx := context.Background()
	for round := 0; round < 2; round++ {
		for i := 0; i < maxLoginFailures-1; i++ {
			_, _ = svc.Login(ctx, account, "wrong-password")
		}
		if _, err := svc.Login(ctx, account, "correct-horse-9"); err != nil {
			t.Fatalf("round %d: correct password after %d failures must work, got %v",
				round+1, maxLoginFailures-1, err)
		}
	}
}

// An expired lock no longer blocks the correct password.
func TestLogin_ExpiredLockAdmits(t *testing.T) {
	svc, repo, account, userID := lockoutSvc(t)
	ctx := context.Background()
	repo.lockedUntil = map[string]time.Time{userID: time.Now().Add(-time.Minute)}
	repo.failures = map[string]int{userID: maxLoginFailures}
	if _, err := svc.Login(ctx, account, "correct-horse-9"); err != nil {
		t.Fatalf("expired lock must admit the correct password, got %v", err)
	}
}

// A completed password reset clears the lockout — the legitimate owner can't
// be locked out of recovery by an attacker's failed attempts.
func TestPasswordReset_ClearsLockout(t *testing.T) {
	svc, repo, account, userID := lockoutSvc(t)
	ctx := context.Background()
	for i := 0; i < maxLoginFailures; i++ {
		_, _ = svc.Login(ctx, account, "wrong-password")
	}
	if _, locked, _ := repo.LoginLockedUntil(ctx, userID); !locked {
		t.Fatal("precondition: account should be locked")
	}
	// Seed a valid reset token directly (same pattern as the 2FA tests).
	raw := "lockout-reset-token"
	if repo.resetTokens == nil {
		repo.resetTokens = map[string]passwordResetTokenRow{}
	}
	repo.resetTokens[sha256Hex(raw)] = passwordResetTokenRow{
		TokenHash: sha256Hex(raw), UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := svc.CompletePasswordReset(ctx, raw, "brand-new-pass-1"); err != nil {
		t.Fatalf("complete reset: %v", err)
	}
	if _, err := svc.Login(ctx, account, "brand-new-pass-1"); err != nil {
		t.Fatalf("login after reset must work (lockout cleared), got %v", err)
	}
}
