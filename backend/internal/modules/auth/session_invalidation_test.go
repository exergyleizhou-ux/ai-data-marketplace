package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRefresh_RejectsTokenIssuedBeforeEpoch is the core enforcement: a refresh
// token minted before the user's session-invalidation epoch is dead.
func TestRefresh_RejectsTokenIssuedBeforeEpoch(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	res, err := svc.Register(ctx, "epoch@example.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// Stamp the epoch in the future relative to the just-issued token.
	for id, u := range repo.byID {
		future := time.Now().Add(time.Hour)
		u.TokensValidAfter = &future
		repo.byID[id] = u
	}
	if _, err := svc.Refresh(ctx, res.Tokens.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh of a token issued before the epoch must fail, got %v", err)
	}
}

// TestCompletePasswordReset_TerminatesExistingSessions is the end-to-end fix: a
// refresh token from a session that began before a password reset must stop
// working once the reset completes (previously it kept working — the revoke hit
// a non-existent table and the error was swallowed).
func TestCompletePasswordReset_TerminatesExistingSessions(t *testing.T) {
	svc, repo, account, userID := lockoutSvc(t)
	ctx := context.Background()

	login, err := svc.Login(ctx, account, "correct-horse-9")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	preResetRefresh := login.Tokens.RefreshToken

	raw := "pwreset-token-xyz"
	if repo.resetTokens == nil {
		repo.resetTokens = map[string]passwordResetTokenRow{}
	}
	repo.resetTokens[sha256Hex(raw)] = passwordResetTokenRow{
		TokenHash: sha256Hex(raw), UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := svc.CompletePasswordReset(ctx, raw, "brand-new-pass-1"); err != nil {
		t.Fatalf("complete reset: %v", err)
	}
	// The pre-reset session can no longer be renewed.
	if _, err := svc.Refresh(ctx, preResetRefresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("pre-reset refresh token must be rejected after reset, got %v", err)
	}
	// And the reset actually stamped the epoch.
	if repo.byID[userID].TokensValidAfter == nil {
		t.Fatal("password reset must set the session-invalidation epoch")
	}
}
