package withdrawal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testRepo(t *testing.T) (Repository, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE withdrawal_requests`)
	return NewRepository(pool), func() { pool.Close() }
}

func seedSeller(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x','seller','verified') RETURNING id::text`,
		"wd-seller-"+fmt.Sprint(uniq)+"@x.com").Scan(&id); err != nil {
		t.Fatalf("seed seller: %v", err)
	}
	return id
}

func TestCreate_PersistsRequest(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	seller := seedSeller(t, repo.(*pgRepo).pool)

	r, err := repo.Create(ctx, Request{SellerID: seller, AmountCents: 10000, Channel: "bank", AccountLabel: "xxx"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Status != StatusPending {
		t.Fatalf("id=%q status=%q", r.ID, r.Status)
	}
}

func TestTransition_PendingToApproved(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)
	_, _ = pool.Exec(ctx, `INSERT INTO users (account, account_type, password_hash, role) VALUES ('ops@x.com','email','x','ops') ON CONFLICT DO NOTHING`)

	r, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 500, Channel: "wechat", AccountLabel: "x"})
	result, err := repo.Transition(ctx, r.ID, StatusPending, StatusApproved, seller, "ok")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusApproved {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestTransition_PendingToRejected(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)
	r, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 100, Channel: "alipay", AccountLabel: "y"})
	result, err := repo.Transition(ctx, r.ID, StatusPending, StatusRejected, seller, "bad docs")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusRejected {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestTransition_ApprovedToCompleted(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)
	r, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 50, Channel: "bank", AccountLabel: "z"})
	repo.Transition(ctx, r.ID, StatusPending, StatusApproved, seller, "")
	result, err := repo.Transition(ctx, r.ID, StatusApproved, StatusCompleted, seller, "sent")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestTransition_FromCompletedReturnsErrBadTransition(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)
	r, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 1, Channel: "wechat", AccountLabel: "w"})
	repo.Transition(ctx, r.ID, StatusPending, StatusApproved, seller, "")
	repo.Transition(ctx, r.ID, StatusApproved, StatusCompleted, seller, "")

	_, err := repo.Transition(ctx, r.ID, StatusCompleted, StatusApproved, seller, "")
	if err != ErrBadTransition {
		t.Fatalf("completed→approved must be ErrBadTransition, got %v", err)
	}
}

func TestSumApprovedAndPending_ExcludesRejectedAndCompleted(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)

	// Create 4 requests with different statuses.
	if _, err := repo.Create(ctx, Request{SellerID: seller, AmountCents: 100, Channel: "bank", AccountLabel: "a"}); err != nil {
		t.Fatal(err)
	}
	r2, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 200, Channel: "wechat", AccountLabel: "b"})
	r3, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 300, Channel: "alipay", AccountLabel: "c"})
	r4, _ := repo.Create(ctx, Request{SellerID: seller, AmountCents: 400, Channel: "bank", AccountLabel: "d"})

	repo.Transition(ctx, r2.ID, StatusPending, StatusApproved, seller, "")
	repo.Transition(ctx, r3.ID, StatusPending, StatusRejected, seller, "nope")
	repo.Transition(ctx, r4.ID, StatusPending, StatusApproved, seller, "")
	repo.Transition(ctx, r4.ID, StatusApproved, StatusCompleted, seller, "done")

	sum, err := repo.SumApprovedAndPending(ctx, seller)
	if err != nil {
		t.Fatal(err)
	}
	// pending(100) + approved(200) = 300; rejected(300) + completed(400) excluded.
	if sum != 300 {
		t.Fatalf("SumApprovedAndPending = %d, want 300", sum)
	}
}

// A completed payout must keep consuming the settled balance, so a second
// withdrawal of the already-paid-out earnings is refused (the treasury-drain fix).
func TestCreateWithinBudget_CountsCompletedPayouts(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	seller := seedSeller(t, repo.(*pgRepo).pool)
	const settled = int64(1000)

	r, err := repo.CreateWithinBudget(ctx, Request{SellerID: seller, AmountCents: 1000, Channel: "bank", AccountLabel: "x"}, settled)
	if err != nil {
		t.Fatalf("first withdrawal of the full balance should succeed: %v", err)
	}
	if _, err := repo.Transition(ctx, r.ID, StatusPending, StatusApproved, seller, "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Transition(ctx, r.ID, StatusApproved, StatusCompleted, seller, "paid"); err != nil {
		t.Fatal(err)
	}
	// The balance is consumed by the COMPLETED payout — re-withdrawal must be refused.
	if _, err := repo.CreateWithinBudget(ctx, Request{SellerID: seller, AmountCents: 1, Channel: "bank", AccountLabel: "x"}, settled); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("re-withdrawal after a completed payout must be refused, got %v", err)
	}
}

// N concurrent withdrawal requests against a balance that fits exactly K must
// insert EXACTLY K and never overshoot — the per-seller advisory lock serializes
// the check-and-insert (without it, concurrent reads both pass and both insert).
func TestCreateWithinBudget_AtomicUnderConcurrency(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seller := seedSeller(t, pool)

	const amt = int64(1000)
	const settled = int64(5000) // fits exactly 5
	const N = 20
	var ok, refused int64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CreateWithinBudget(ctx, Request{SellerID: seller, AmountCents: amt, Channel: "bank", AccountLabel: "x"}, settled)
			switch {
			case err == nil:
				atomic.AddInt64(&ok, 1)
			case errors.Is(err, ErrInsufficientBalance):
				atomic.AddInt64(&refused, 1)
			default:
				t.Errorf("unexpected CreateWithinBudget error: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok != 5 {
		t.Errorf("committed withdrawals = %d, want 5 — the advisory lock failed to serialize", ok)
	}
	var total int64
	pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents),0) FROM withdrawal_requests WHERE seller_id=$1`, seller).Scan(&total)
	if total > settled {
		t.Errorf("total requested %d OVERSHOT settled balance %d under concurrency", total, settled)
	}
}
