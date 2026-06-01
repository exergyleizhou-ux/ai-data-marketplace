package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInMemoryDenylist(t *testing.T) {
	ctx := context.Background()
	dl := NewInMemoryDenylist()

	// Unknown / empty jti is never revoked.
	if rev, _ := dl.IsRevoked(ctx, "nope"); rev {
		t.Fatal("unknown jti reported revoked")
	}
	if rev, _ := dl.IsRevoked(ctx, ""); rev {
		t.Fatal("empty jti reported revoked")
	}

	// Revoked jti reports revoked.
	if err := dl.Revoke(ctx, "abc", time.Hour); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if rev, _ := dl.IsRevoked(ctx, "abc"); !rev {
		t.Fatal("revoked jti not reported revoked")
	}

	// ttl <= 0 is a no-op (token already expired).
	if err := dl.Revoke(ctx, "expired", -time.Second); err != nil {
		t.Fatalf("revoke expired: %v", err)
	}
	if rev, _ := dl.IsRevoked(ctx, "expired"); rev {
		t.Fatal("zero-ttl revoke should be a no-op")
	}
}

func newTestServiceWithDenylist(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	return NewService(repo, tm, WithDenylist(NewInMemoryDenylist())), repo
}

func TestRefreshRotationRejectsReuse(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceWithDenylist(t)

	reg, err := svc.Register(ctx, "alice@example.com", "email", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	first := reg.Tokens.RefreshToken

	// First refresh succeeds and rotates to a new refresh token.
	rotated, err := svc.Refresh(ctx, first)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if rotated.Tokens.RefreshToken == first {
		t.Fatal("refresh did not rotate the token")
	}

	// Reusing the original (now-rotated) token is rejected.
	if _, err := svc.Refresh(ctx, first); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("reused refresh token: got %v, want ErrInvalidToken", err)
	}

	// The rotated token still works (single use).
	if _, err := svc.Refresh(ctx, rotated.Tokens.RefreshToken); err != nil {
		t.Fatalf("rotated token should work once: %v", err)
	}
}

func TestLogoutRevokesRefresh(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceWithDenylist(t)

	reg, err := svc.Register(ctx, "bob@example.com", "email", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	rt := reg.Tokens.RefreshToken

	if err := svc.Logout(ctx, rt); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.Refresh(ctx, rt); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh after logout: got %v, want ErrInvalidToken", err)
	}

	// Idempotent: logging out again, or with garbage, is a success no-op.
	if err := svc.Logout(ctx, rt); err != nil {
		t.Fatalf("second logout should be no-op: %v", err)
	}
	if err := svc.Logout(ctx, "not-a-token"); err != nil {
		t.Fatalf("logout of invalid token should be no-op: %v", err)
	}
}

// Without a denylist the service stays stateless: rotation is a no-op so an
// unexpired refresh token keeps working (pre-H4 behaviour, backward compatible).
func TestRefreshNoDenylistStillStateless(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	svc := NewService(repo, tm) // default noop denylist

	reg, err := svc.Register(ctx, "carol@example.com", "email", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	rt := reg.Tokens.RefreshToken
	for i := 0; i < 3; i++ {
		if _, err := svc.Refresh(ctx, rt); err != nil {
			t.Fatalf("refresh %d without denylist: %v", i, err)
		}
	}
}
