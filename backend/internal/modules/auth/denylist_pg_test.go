package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testDenylistPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return pool, func() { pool.Close() }
}

func freshJTI(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen jti: %v", err)
	}
	return id
}

func TestPostgresDenylist_RevokeThenIsRevoked(t *testing.T) {
	pool, cleanup := testDenylistPool(t)
	defer cleanup()
	ctx := context.Background()
	d := NewPostgresDenylist(pool)
	jti := freshJTI(t, ctx, pool)

	if r, err := d.IsRevoked(ctx, jti); err != nil || r {
		t.Fatalf("fresh jti: revoked=%v err=%v, want false/nil", r, err)
	}
	if err := d.Revoke(ctx, jti, time.Hour); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if r, err := d.IsRevoked(ctx, jti); err != nil || !r {
		t.Fatalf("after revoke: revoked=%v err=%v, want true/nil", r, err)
	}

	// ttl <= 0 is a no-op: an already-expired token need not be stored.
	jti2 := freshJTI(t, ctx, pool)
	if err := d.Revoke(ctx, jti2, 0); err != nil {
		t.Fatalf("revoke ttl=0: %v", err)
	}
	if r, _ := d.IsRevoked(ctx, jti2); r {
		t.Fatal("ttl<=0 must be a no-op (not revoked)")
	}
}

// The core regression: revocation must survive across server instances (and
// restarts). The in-memory fallback fails this — a logout on one instance
// leaves the token valid on another. Postgres is durable + shared.
func TestPostgresDenylist_DurableAcrossInstances(t *testing.T) {
	pool, cleanup := testDenylistPool(t)
	defer cleanup()
	ctx := context.Background()
	jti := freshJTI(t, ctx, pool)

	instanceA := NewPostgresDenylist(pool)
	if err := instanceA.Revoke(ctx, jti, time.Hour); err != nil {
		t.Fatalf("revoke on A: %v", err)
	}
	// A different denylist object models a different server instance reading
	// the same shared store.
	instanceB := NewPostgresDenylist(pool)
	if r, err := instanceB.IsRevoked(ctx, jti); err != nil || !r {
		t.Fatalf("instance B sees revocation = %v (err %v), want true — revocation must be durable/shared", r, err)
	}
}
