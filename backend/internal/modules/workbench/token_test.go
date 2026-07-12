package workbench

import (
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestTokenClaimsAndTTL(t *testing.T) {
	m := NewTokenManager("separate-workbench-secret", 5*time.Minute)
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	raw, ttl, err := m.Issue("user-1", "workspace-1")
	if err != nil || ttl != 300 {
		t.Fatalf("issue: ttl=%d err=%v", ttl, err)
	}
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return []byte("separate-workbench-secret"), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("oasis"), jwt.WithAudience("lumen-workbench"), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !tok.Valid {
		t.Fatal(err)
	}
	if c.UID != "user-1" || c.Subject != "user-1" || c.WorkspaceID != "workspace-1" || len(c.ID) != 32 || len(c.Permissions) != 6 {
		t.Fatalf("bad claims: %+v", c)
	}
}

func TestTokenTTLCapped(t *testing.T) {
	if NewTokenManager("x", time.Hour).ttl != 5*time.Minute {
		t.Fatal("TTL not capped")
	}
}
