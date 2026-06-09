package dataset

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testPool(t *testing.T) (Repository, func()) {
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
	// Clean up quality_retries before test so previous runs don't pollute.
	pool.Exec(context.Background(), `DELETE FROM quality_retries`)
	return NewRepository(pool), func() { pool.Close() }
}

func seedDataset(t *testing.T, pool *pgxpool.Pool, dsID string) string {
	t.Helper()
	var sellerID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role) VALUES ($1, 'email', 'x', 'seller') ON CONFLICT (account) DO UPDATE SET role='seller' RETURNING id::text`,
		"test-"+dsID[:8]+"@x.com").Scan(&sellerID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		`INSERT INTO datasets (id, seller_id, title, data_type, license_type) VALUES ($1, $2, 'test', 'text', 'commercial') ON CONFLICT (id) DO NOTHING`,
		dsID, sellerID)
	if err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	return sellerID
}

func TestEnqueueQualityRetry_InsertThenUpsert(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	_ = seedDataset(t, pool, "00000000-0000-0000-0000-000000100001")

	if err := repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100001", "00000000-0000-0000-0000-0000000000a1", "sha-a", 3); err != nil {
		t.Fatal(err)
	}
	// Second enqueue should reset attempts and clear last_error.
	if err := repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100001", "00000000-0000-0000-0000-0000000000a2", "sha-b", 5); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListDueQualityRetries(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Attempts != 0 {
		t.Fatalf("attempts = %d, want 0 (reset on upsert)", rows[0].Attempts)
	}
	if rows[0].LastError != "" {
		t.Fatalf("last_error = %q, want empty", rows[0].LastError)
	}
	if rows[0].MaxAttempts != 5 {
		t.Fatalf("max_attempts = %d, want 5", rows[0].MaxAttempts)
	}
	if rows[0].ContentSHA256 != "sha-b" {
		t.Fatalf("content_sha = %q, want sha-b", rows[0].ContentSHA256)
	}
}

func TestListDueQualityRetries_OnlyReturnsDueRows(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100002")
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100003")
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100004")

	// Two rows are due now.
	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100002", "00000000-0000-0000-0000-0000000000a1", "sha-1", 3)
	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100003", "00000000-0000-0000-0000-0000000000a1", "sha-2", 3)
	// One row with next_at in the future.
	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100004", "00000000-0000-0000-0000-0000000000a1", "sha-3", 3)
	pool.Exec(ctx, `UPDATE quality_retries SET next_at = now() + interval '1 hour' WHERE dataset_id = '00000000-0000-0000-0000-000000100004'`)

	rows, err := repo.ListDueQualityRetries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.DatasetID == "00000000-0000-0000-0000-000000100004" {
			t.Fatal("future row must not be returned")
		}
	}
}

func TestListDueQualityRetries_ExcludesMaxedOut(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100005")

	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100005", "00000000-0000-0000-0000-0000000000a1", "sha", 3)
	// Manually exhaust attempts.
	pool.Exec(ctx, `UPDATE quality_retries SET attempts = 3, next_at = now() - interval '1 hour' WHERE dataset_id = '00000000-0000-0000-0000-000000100005'`)

	rows, err := repo.ListDueQualityRetries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.DatasetID == "00000000-0000-0000-0000-000000100005" {
			t.Fatal("maxed-out row must not be returned")
		}
	}
}

func TestMarkQualityRetryAttempt_IncrementsAndUpdatesNextAt(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100006")

	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100006", "00000000-0000-0000-0000-0000000000a1", "sha", 3)
	nextAt := time.Now().Add(120 * time.Second)
	if err := repo.MarkQualityRetryAttempt(ctx, "00000000-0000-0000-0000-000000100006", nextAt, "oom"); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListDueQualityRetries(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatal("just-marked row has future next_at, must not appear as due")
	}

	// Verify via direct query.
	var attempts int
	var gotErr string
	if err := pool.QueryRow(ctx,
		`SELECT attempts, COALESCE(last_error, '') FROM quality_retries WHERE dataset_id = '00000000-0000-0000-0000-000000100006'`).
		Scan(&attempts, &gotErr); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if gotErr != "oom" {
		t.Fatalf("last_error = %q, want oom", gotErr)
	}
}

func TestDeleteQualityRetry_RemovesRow(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedDataset(t, pool, "00000000-0000-0000-0000-000000100007")

	repo.EnqueueQualityRetry(ctx, "00000000-0000-0000-0000-000000100007", "00000000-0000-0000-0000-0000000000a1", "sha", 3)
	if err := repo.DeleteQualityRetry(ctx, "00000000-0000-0000-0000-000000100007"); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.ListDueQualityRetries(ctx, 10)
	for _, r := range rows {
		if r.DatasetID == "00000000-0000-0000-0000-000000100007" {
			t.Fatal("deleted row must not be returned")
		}
	}
}
