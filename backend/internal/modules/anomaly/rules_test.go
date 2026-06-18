package anomaly

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// testRulesPool returns a live pool for exercising the anomaly rule SQL.
// The rule queries are real SQL and were never covered by a DB-backed test,
// which is how a malformed ARRAY_AGG(... LIMIT) reached production silently.
func testRulesPool(t *testing.T) (*pgxpool.Pool, func()) {
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

// newActor mints a unique actor_id so each test asserts only on its own rows.
// audit_logs is append-only (a trigger blocks DELETE/UPDATE), so isolation
// comes from a fresh actor rather than truncation.
func newActor(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen actor: %v", err)
	}
	return id
}

func TestRepeatedFailureRule_DetectsAndCapsSamples(t *testing.T) {
	pool, cleanup := testRulesPool(t)
	defer cleanup()
	ctx := context.Background()
	actor := newActor(t, ctx, pool)
	since := time.Now().UTC().Add(-time.Minute)

	// 10 failure-type actions by the same actor -> meets HAVING COUNT(*) >= 10.
	for i := 0; i < 10; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, created_at)
			 VALUES ($1, 'dataset.reject', 'dataset', $2, now())`,
			actor, fmt.Sprintf("r%d", i)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	rule := &RepeatedFailureRule{}
	got, err := rule.Detect(ctx, pool, since)
	if err != nil {
		t.Fatalf("Detect must not error (SQL regression): %v", err)
	}

	mine := findByActor(got, actor)
	if mine == nil {
		t.Fatalf("expected an anomaly for actor %s, got %d anomalies", actor, len(got))
	}
	if mine.Count != 10 {
		t.Fatalf("count = %d, want 10", mine.Count)
	}
	// 10 rows, but samples must be capped at the most-recent 5.
	if len(mine.SampleAuditIDs) != 5 {
		t.Fatalf("sample ids = %d, want capped at 5", len(mine.SampleAuditIDs))
	}
}

func TestBulkModificationRule_DetectsAndCapsSamples(t *testing.T) {
	pool, cleanup := testRulesPool(t)
	defer cleanup()
	ctx := context.Background()
	actor := newActor(t, ctx, pool)
	since := time.Now().UTC().Add(-time.Minute)

	// 20 distinct resources modified by the same actor+action+resource_type
	// -> meets HAVING COUNT(DISTINCT resource_id) >= 20. Action does not match
	// the repeated_failure patterns, so it only triggers this rule.
	for i := 0; i < 20; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, created_at)
			 VALUES ($1, 'dataset.update', 'dataset', $2, now())`,
			actor, fmt.Sprintf("res%d", i)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	rule := &BulkModificationRule{}
	got, err := rule.Detect(ctx, pool, since)
	if err != nil {
		t.Fatalf("Detect must not error (SQL regression): %v", err)
	}

	mine := findByActor(got, actor)
	if mine == nil {
		t.Fatalf("expected an anomaly for actor %s, got %d anomalies", actor, len(got))
	}
	if mine.Count != 20 {
		t.Fatalf("count = %d, want 20", mine.Count)
	}
	if len(mine.SampleAuditIDs) != 5 {
		t.Fatalf("sample ids = %d, want capped at 5", len(mine.SampleAuditIDs))
	}
}

func findByActor(items []Anomaly, actor string) *Anomaly {
	for i := range items {
		if items[i].ActorID == actor {
			return &items[i]
		}
	}
	return nil
}
