package auditlog

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
	// audit_logs append-only trigger blocks UPDATE/DELETE, but INSERT + TRUNCATE are allowed.
	pool.Exec(context.Background(), `TRUNCATE audit_logs`)
	return NewRepository(pool), func() { pool.Close() }
}

func insertLog(t *testing.T, pool *pgxpool.Pool, actorID, action, resourceType, resourceID string, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, created_at)
		 VALUES ($1::uuid, $2, $3, $4, $5)`,
		actorID, action, resourceType, resourceID, createdAt)
	if err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}
}

func TestList_NoFilter_ReturnsAllRecent(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	base := time.Now().UTC().Add(-time.Hour)
	insertLog(t, repo.(*pgRepo).pool, "00000000-0000-0000-0000-000000000001", "user.login", "user", "", base)
	insertLog(t, repo.(*pgRepo).pool, "00000000-0000-0000-0000-000000000002", "order.create", "order", "", base.Add(time.Second))
	insertLog(t, repo.(*pgRepo).pool, "00000000-0000-0000-0000-000000000003", "dataset.create", "dataset", "", base.Add(2*time.Second))

	items, err := repo.List(context.Background(), ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d, want 3", len(items))
	}
	// created_at DESC -> latest first.
	if items[0].Action != "dataset.create" {
		t.Fatalf("first action = %q, want dataset.create", items[0].Action)
	}
	if items[2].Action != "user.login" {
		t.Fatalf("last action = %q, want user.login", items[2].Action)
	}
}

func TestList_FiltersByAction(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 2; i++ {
		insertLog(t, pool, "00000000-0000-0000-0000-100000000000", "foo", "x", "", base)
	}
	for i := 0; i < 3; i++ {
		insertLog(t, pool, "00000000-0000-0000-0000-100000000001", "bar", "x", "", base)
	}

	items, err := repo.List(context.Background(), ListFilter{Action: "bar", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d, want 3", len(items))
	}
	for _, e := range items {
		if e.Action != "bar" {
			t.Fatalf("leaked action %q", e.Action)
		}
	}
}

func TestList_FiltersByActor(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	base := time.Now().UTC().Add(-time.Hour)
	insertLog(t, pool, "00000000-0000-0000-0000-a00000000001", "act", "x", "", base)
	insertLog(t, pool, "00000000-0000-0000-0000-b00000000001", "act", "x", "", base)

	items, err := repo.List(context.Background(), ListFilter{
		ActorID: "00000000-0000-0000-0000-a00000000001", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d, want 1", len(items))
	}
	if items[0].ActorID != "00000000-0000-0000-0000-a00000000001" {
		t.Fatalf("actor = %q, want a", items[0].ActorID)
	}
}

func TestList_FiltersByResource(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	base := time.Now().UTC().Add(-time.Hour)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000001", "a", "order", "o1", base)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000002", "a", "order", "o2", base)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000003", "a", "dataset", "d1", base)

	items, err := repo.List(context.Background(), ListFilter{
		ResourceType: "order", ResourceID: "o1", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d, want 1", len(items))
	}
	if items[0].ResourceID != "o1" {
		t.Fatalf("resourceID = %q, want o1", items[0].ResourceID)
	}
}

func TestList_FiltersByDateRange(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	t1 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC)

	insertLog(t, pool, "00000000-0000-0000-0000-000000000001", "a", "x", "", t1)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000002", "b", "x", "", t2)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000003", "c", "x", "", t3)

	// Range [t1, t3) should include t1 and t2, excluding t3.
	items, err := repo.List(context.Background(), ListFilter{
		From:  t1.Format(time.RFC3339),
		To:    t3.Format(time.RFC3339),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d, want 2 (t1 and t2, not t3)", len(items))
	}
}

func TestList_LimitClampedTo200(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
		insertLog(t, pool, id, "act", "x", "", base)
	}

	items, err := repo.List(context.Background(), ListFilter{Limit: 500})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) > 200 {
		t.Fatalf("got %d, limit must be clamped to 200 max", len(items))
	}
}

func TestList_NegativeOffsetClampedToZero(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	pool := repo.(*pgRepo).pool
	base := time.Now().UTC().Add(-time.Hour)
	insertLog(t, pool, "00000000-0000-0000-0000-000000000001", "act", "x", "", base)

	items, err := repo.List(context.Background(), ListFilter{Limit: 10, Offset: -5})
	if err != nil {
		t.Fatalf("list with negative offset must not error, got: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d, want 1 (negative offset clamped to 0)", len(items))
	}
}
