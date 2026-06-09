package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testOutboxRepo(t *testing.T) (OutboxRepository, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE settlement_outbox, orders CASCADE`)
	return NewOutboxRepository(pool), func() { pool.Close() }
}

func seedOrderForOutbox(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	suf := make([]byte, 3)
	_, _ = rand.Read(suf)
	s := hex.EncodeToString(suf)

	pool.Exec(ctx, `INSERT INTO users (id, account, account_type, password_hash, role)
		VALUES ('00000000-0000-0000-0000-000000000001', 'ob-test@x.com','email','x','buyer') ON CONFLICT DO NOTHING`)
	pool.Exec(ctx, `INSERT INTO users (id, account, account_type, password_hash, role)
		VALUES ('00000000-0000-0000-0000-000000000002', 'ob-seller@x.com','email','x','seller') ON CONFLICT DO NOTHING`)
	dsID := fmt.Sprintf("30000000-0000-0000-0000-%012s", s)
	pool.Exec(ctx, `INSERT INTO datasets (id, seller_id, title, data_type, license_type, status)
		VALUES ($1,'00000000-0000-0000-0000-000000000002','ob-ds','text','commercial','published') ON CONFLICT DO NOTHING`,
		dsID)
	var oid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO orders (buyer_id, seller_id, dataset_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type)
		 VALUES ('00000000-0000-0000-0000-000000000001'::uuid,'00000000-0000-0000-0000-000000000002'::uuid,
		 $1::uuid,'commercial',100,10,90,'created','download')
		 RETURNING id::text`, dsID).Scan(&oid); err != nil {
		t.Fatal(err)
	}
	return oid
}

func TestPgOutbox_ListOutbox_FiltersByStatus(t *testing.T) {
	repo, cleanup := testOutboxRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgOutboxRepo).pool

	for i := 0; i < 3; i++ {
		oid := seedOrderForOutbox(t, pool)
		pool.Exec(ctx, `INSERT INTO settlement_outbox (order_id, status) VALUES ($1, 'failed')`, oid)
	}
	for i := 0; i < 2; i++ {
		oid := seedOrderForOutbox(t, pool)
		pool.Exec(ctx, `INSERT INTO settlement_outbox (order_id, status) VALUES ($1, 'pending')`, oid)
	}

	items, err := repo.ListOutbox(ctx, "failed", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("failed items = %d, want 3", len(items))
	}
	for _, e := range items {
		if e.Status != "failed" {
			t.Fatalf("leaked non-failed status: %q", e.Status)
		}
	}
}

func TestPgOutbox_ListOutbox_OrdersByUpdatedAtDesc(t *testing.T) {
	repo, cleanup := testOutboxRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgOutboxRepo).pool

	for i := 0; i < 3; i++ {
		oid := seedOrderForOutbox(t, pool)
		pool.Exec(ctx, `INSERT INTO settlement_outbox (order_id, status) VALUES ($1, 'pending')`, oid)
	}
	items, err := repo.ListOutbox(ctx, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("need at least 2 items to test ordering, got %d", len(items))
	}
	if items[0].UpdatedAt < items[1].UpdatedAt {
		t.Fatal("ListOutbox must order by updated_at DESC")
	}
}

func TestPgOutbox_RetryOutbox_FailedToPending(t *testing.T) {
	repo, cleanup := testOutboxRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgOutboxRepo).pool

	oid := seedOrderForOutbox(t, pool)
	pool.Exec(ctx, `INSERT INTO settlement_outbox (order_id, status) VALUES ($1, 'failed')`, oid)

	if err := repo.RetryOutbox(ctx, oid); err != nil {
		t.Fatal(err)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM settlement_outbox WHERE order_id=$1`, oid).Scan(&status)
	if status != "pending" {
		t.Fatalf("status after Retry = %q, want pending", status)
	}
}

func TestPgOutbox_RetryOutbox_NonFailedReturnsErrOutboxNotFailed(t *testing.T) {
	repo, cleanup := testOutboxRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgOutboxRepo).pool

	oid := seedOrderForOutbox(t, pool)
	pool.Exec(ctx, `INSERT INTO settlement_outbox (order_id, status) VALUES ($1, 'pending')`, oid)

	err := repo.RetryOutbox(ctx, oid)
	if err != ErrOutboxNotFailed {
		t.Fatalf("RetryOutbox on pending must return ErrOutboxNotFailed, got %v", err)
	}
}
