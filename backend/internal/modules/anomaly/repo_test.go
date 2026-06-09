package anomaly

import (
	"context"
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
	pool.Exec(context.Background(), `TRUNCATE TABLE audit_anomalies`)
	return NewRepository(pool), func() { pool.Close() }
}

func TestUpsert_ResolvedAnomalyNotUpdated(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	now := time.Now().UTC()
	a1 := Anomaly{
		Kind: "high_risk_action", ActorID: "", ResourcePattern: "dataset:ds1",
		SampleAuditIDs: []int64{1}, Count: 1,
		FirstSeenAt: now.Format(time.RFC3339), LastSeenAt: now.Format(time.RFC3339),
	}
	if err := repo.Upsert(ctx, a1); err != nil {
		t.Fatal(err)
	}
	// Mark as resolved.
	var id string
	if err := pool.QueryRow(ctx,
		`UPDATE audit_anomalies SET status='resolved' WHERE kind='high_risk_action'
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	// Upsert same key — since resolved anomaly is not 'open', no conflict.
	// A NEW open row is created.
	a2 := Anomaly{
		Kind: "high_risk_action", ActorID: "", ResourcePattern: "dataset:ds1",
		SampleAuditIDs: []int64{2, 3}, Count: 10,
		FirstSeenAt: now.Format(time.RFC3339), LastSeenAt: now.Add(time.Hour).Format(time.RFC3339),
	}
	if err := repo.Upsert(ctx, a2); err != nil {
		t.Fatal(err)
	}

	// Resolved row untouched.
	var gotCount int
	var gotStatus string
	pool.QueryRow(ctx, `SELECT count, status FROM audit_anomalies WHERE id=$1`, id).
		Scan(&gotCount, &gotStatus)
	if gotCount != 1 {
		t.Fatalf("resolved row count unchanged: got %d, want 1", gotCount)
	}
	if gotStatus != "resolved" {
		t.Fatalf("status must stay resolved: got %q", gotStatus)
	}

	// And a new open row exists with count=10.
	var newCount int
	pool.QueryRow(ctx,
		`SELECT count FROM audit_anomalies WHERE kind='high_risk_action' AND status='open'`).
		Scan(&newCount)
	if newCount != 10 {
		t.Fatalf("new open row count = %d, want 10", newCount)
	}
}

func TestSetStatus_TransitionsOpenToAcknowledged(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()

	a := Anomaly{
		Kind: "repeated_failure", ActorID: "00000000-0000-0000-0000-000000000000",
		ResourcePattern: "order.reject", SampleAuditIDs: []int64{1, 2}, Count: 10,
		FirstSeenAt: now.Format(time.RFC3339), LastSeenAt: now.Format(time.RFC3339),
	}
	if err := repo.Upsert(ctx, a); err != nil {
		t.Fatal(err)
	}

	items, _ := repo.List(ctx, "open", 1, 0)
	if len(items) == 0 {
		t.Fatal("expected open anomaly after upsert")
	}

	result, err := repo.SetStatus(ctx, items[0].ID, "acknowledged", "ops-user", "looking")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "acknowledged" {
		t.Fatalf("status = %q, want acknowledged", result.Status)
	}
}
