//go:build integration

// Package order_test concurrency integration tests — opt-in, require a real
// Postgres (the duplicate-order / atomic-transition guarantees are DB-enforced
// via the uniq_orders_active_per_buyer_dataset index and atomic UPDATE…WHERE
// status, which the in-memory fakeRepo cannot exercise).
//
//	Run:  TEST_DATABASE_URL='postgres://postgres@localhost:55432/postgres?sslmode=disable' \
//	      go test -tags=integration -run TestConcurrent ./internal/modules/order/
//
// Without TEST_DATABASE_URL the tests skip, so normal `go test ./...` and CI
// are unaffected unless they opt in with -tags=integration + a DB.
package order_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/order"
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

func seedOrderParty(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (buyer, seller, dataset, version string) {
	t.Helper()
	mustID := func(q string, args ...any) string {
		var id string
		if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
			t.Fatalf("seed (%s): %v", q, err)
		}
		return id
	}
	// gen_random_uuid in the account keeps reruns collision-free.
	buyer = mustID(`INSERT INTO users (account, account_type, password_hash)
		VALUES ('buyer-'||gen_random_uuid()||'@t.test','email','x') RETURNING id`)
	seller = mustID(`INSERT INTO users (account, account_type, password_hash)
		VALUES ('seller-'||gen_random_uuid()||'@t.test','email','x') RETURNING id`)
	dataset = mustID(`INSERT INTO datasets (seller_id, title, data_type, license_type)
		VALUES ($1,'concurrency-fixture','text','commercial') RETURNING id`, seller)
	version = mustID(`INSERT INTO dataset_versions (dataset_id, version_no)
		VALUES ($1, 1) RETURNING id`, dataset)
	return
}

// TestConcurrentCreate_OnlyOneActiveOrder fires N simultaneous Create calls for
// the SAME buyer+dataset. The unique partial index must let exactly one through
// and reject the rest as ErrDuplicateOrder — i.e. no double-spend race.
func TestConcurrentCreate_OnlyOneActiveOrder(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	buyer, seller, dataset, version := seedOrderParty(t, ctx, pool)

	repo := order.NewRepository(pool)
	o := order.Order{
		BuyerID: buyer, SellerID: seller, DatasetID: dataset, VersionID: version,
		LicenseType: "commercial", AmountCents: 1000, PlatformFeeCents: 100, SellerAmountCents: 900,
	}

	const n = 16
	var ok, dup, other int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximize the race
			_, err := repo.Create(ctx, o)
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, order.ErrDuplicateOrder):
				atomic.AddInt32(&dup, 1)
			default:
				atomic.AddInt32(&other, 1)
				t.Errorf("unexpected create error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok != 1 {
		t.Fatalf("want exactly 1 active order created, got %d (dup=%d other=%d)", ok, dup, other)
	}
	if dup != n-1 {
		t.Fatalf("want %d duplicate rejections, got %d", n-1, dup)
	}
}

// TestConcurrentTransition_AtomicOnce fires N simultaneous created->paid
// transitions on one order. The atomic UPDATE…WHERE status='created' must let
// exactly one win; the losers see ErrBadTransition (status already moved).
func TestConcurrentTransition_AtomicOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	buyer, seller, dataset, version := seedOrderParty(t, ctx, pool)

	repo := order.NewRepository(pool)
	created, err := repo.Create(ctx, order.Order{
		BuyerID: buyer, SellerID: seller, DatasetID: dataset, VersionID: version,
		LicenseType: "commercial", AmountCents: 1000, PlatformFeeCents: 100, SellerAmountCents: 900,
	})
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}

	const n = 16
	var ok, bad, other int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.Transition(ctx, created.ID, "created", "paid", false)
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, order.ErrBadTransition):
				atomic.AddInt32(&bad, 1)
			default:
				atomic.AddInt32(&other, 1)
				t.Errorf("unexpected transition error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok != 1 {
		t.Fatalf("want exactly 1 winning transition, got %d (bad=%d other=%d)", ok, bad, other)
	}
}
