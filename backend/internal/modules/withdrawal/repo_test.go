package withdrawal

import (
	"context"
	"fmt"
	"os"
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
