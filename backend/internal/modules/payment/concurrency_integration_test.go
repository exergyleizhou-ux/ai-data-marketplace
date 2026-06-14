//go:build integration

// Package payment_test concurrency integration test — opt-in, needs a real
// Postgres. The "no double-split" guarantee is DB-enforced (settlements.order_id
// UNIQUE + idempotency_key UNIQUE); the in-memory fakes can't exercise the race.
//
//	Run:  TEST_DATABASE_URL='postgres://postgres@localhost:55433/postgres?sslmode=disable' \
//	      go test -tags=integration -run TestConcurrentSettle ./internal/modules/payment/
package payment_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/payment"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run pg concurrency integration tests")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return pool
}

// seedConfirmedOrder inserts users→dataset→version→order and returns the order id.
func seedConfirmedOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := func(q string, args ...any) string {
		var out string
		if err := pool.QueryRow(ctx, q, args...).Scan(&out); err != nil {
			t.Fatalf("seed (%s): %v", q, err)
		}
		return out
	}
	buyer := id(`INSERT INTO users (account, account_type, password_hash)
		VALUES ('b-'||gen_random_uuid()||'@t.test','email','x') RETURNING id`)
	seller := id(`INSERT INTO users (account, account_type, password_hash)
		VALUES ('s-'||gen_random_uuid()||'@t.test','email','x') RETURNING id`)
	dataset := id(`INSERT INTO datasets (seller_id, title, data_type, license_type)
		VALUES ($1,'fixture','text','commercial') RETURNING id`, seller)
	version := id(`INSERT INTO dataset_versions (dataset_id, version_no)
		VALUES ($1,1) RETURNING id`, dataset)
	return id(`INSERT INTO orders (buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status)
		VALUES ($1,$2,$3,$4,'commercial',1000,100,900,'confirmed') RETURNING id`,
		buyer, seller, dataset, version)
}

// TestConcurrentSettle_SingleSplit fires N simultaneous CreateSettlement calls
// for one order. Exactly one must win (created=true); the rest must be rejected
// idempotently (created=false) — no double payout.
func TestConcurrentSettle_SingleSplit(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	orderID := seedConfirmedOrder(t, ctx, pool)

	repo := payment.NewRepository(pool)
	key := "settle-" + orderID

	const n = 16
	var won, idem, fail int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			created, err := repo.CreateSettlement(ctx, orderID, key, 100, 900)
			switch {
			case err != nil:
				atomic.AddInt32(&fail, 1)
				t.Errorf("unexpected settle error: %v", err)
			case created:
				atomic.AddInt32(&won, 1)
			default:
				atomic.AddInt32(&idem, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if won != 1 {
		t.Fatalf("want exactly 1 settlement created, got %d (idempotent=%d fail=%d)", won, idem, fail)
	}
	if idem != n-1 {
		t.Fatalf("want %d idempotent rejections, got %d", n-1, idem)
	}
}
