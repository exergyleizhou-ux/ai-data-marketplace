package apikey

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func seedUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	acct := fmt.Sprintf("vt-%d@verify.test", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status, status)
		 VALUES ($1,'email','x','buyer','none','active') RETURNING id::text`, acct).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// TestAPIKeyLifecycle exercises the metered-key contract end-to-end on a real DB:
// issue → authenticate (metered) → quota enforced → month resets → revoke.
func TestAPIKeyLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)
	svc := NewService(repo)
	acct := seedUser(t, pool)

	// Issue a free-tier key (quota 5/month).
	k, plain, err := svc.Issue(ctx, acct, "my key", "free")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if plain[:8] != "sk_live_" || k.Tier != "free" {
		t.Fatalf("unexpected issue: plain=%.8q tier=%q", plain, k.Tier)
	}

	const month = "2026-06"
	for i := 1; i <= Tiers["free"].MonthlyQuota; i++ {
		got, err := repo.AuthenticateAndMeter(ctx, HashKey(plain), month)
		if err != nil {
			t.Fatalf("auth #%d: %v", i, err)
		}
		if got.UsageCount != i {
			t.Fatalf("auth #%d usage=%d, want %d", i, got.UsageCount, i)
		}
	}
	// One past the free quota → rejected.
	if _, err := repo.AuthenticateAndMeter(ctx, HashKey(plain), month); err != ErrQuotaExceeded {
		t.Fatalf("over-quota: err=%v, want ErrQuotaExceeded", err)
	}
	// A new month resets the counter.
	got, err := repo.AuthenticateAndMeter(ctx, HashKey(plain), "2026-07")
	if err != nil || got.UsageCount != 1 {
		t.Fatalf("new month: err=%v usage=%d, want nil/1", err, got.UsageCount)
	}

	// An unknown key is rejected.
	if _, err := svc.Authenticate(ctx, "sk_live_bogus"); err != ErrInvalidKey {
		t.Fatalf("bogus key: err=%v, want ErrInvalidKey", err)
	}

	// Revoke → the key no longer authenticates.
	if err := svc.Revoke(ctx, acct, k.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := repo.AuthenticateAndMeter(ctx, HashKey(plain), month); err != ErrInvalidKey {
		t.Fatalf("revoked key: err=%v, want ErrInvalidKey", err)
	}

	// List shows the (revoked) key.
	keys, err := svc.List(ctx, acct)
	if err != nil || len(keys) != 1 || !keys[0].Revoked() {
		t.Fatalf("list: err=%v keys=%+v", err, keys)
	}
}

// TestSetTier: a subscription change upgrades all of an account's active keys,
// lifting their quota (the billing target a Stripe webhook calls).
func TestSetTier(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	svc := NewService(NewRepository(pool))
	acct := seedUser(t, pool)

	_, plain, _ := svc.Issue(ctx, acct, "k", "free")
	n, err := svc.SetTier(ctx, acct, "pro")
	if err != nil || n != 1 {
		t.Fatalf("set tier: n=%d err=%v", n, err)
	}
	// Pro quota (500) now applies — a 6th scan (over the old free 5) succeeds.
	const month = "2026-08"
	for i := 0; i < 6; i++ {
		if _, err := svc.repo.AuthenticateAndMeter(ctx, HashKey(plain), month); err != nil {
			t.Fatalf("scan #%d after upgrade: %v", i+1, err)
		}
	}
}
