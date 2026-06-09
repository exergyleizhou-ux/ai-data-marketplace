package compliance

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testDeletionRepo(t *testing.T) (DeletionRepository, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE account_deletion_requests`)
	return NewDeletionRepository(pool), func() { pool.Close() }
}

func seedDelUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','buyer') RETURNING id::text`,
		"del-"+fmt.Sprint(uniq)+"@x.com").Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestDeletionRepo_UniquePerUser(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	_, err := repo.Create(ctx, uid, "reason", time.Now().Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Second Create should upsert (ON CONFLICT DO UPDATE).
	_, err = repo.Create(ctx, uid, "reason2", time.Now().Add(7*24*time.Hour))
	if err != nil {
		t.Fatal("second Create must upsert, not error", err)
	}
}

func TestDeletionRepo_Transition_CoolingToApproved(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	d, _ := repo.Create(ctx, uid, "reason", time.Now().Add(7*24*time.Hour))
	r, err := repo.Transition(ctx, d.ID, DeletionCooling, DeletionApproved, uid, "approved")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != DeletionApproved {
		t.Fatalf("status = %q", r.Status)
	}
}

func TestDeletionRepo_Transition_CoolingToCancelled(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	d, _ := repo.Create(ctx, uid, "reason", time.Now().Add(7*24*time.Hour))
	r, err := repo.Transition(ctx, d.ID, DeletionCooling, DeletionCancelled, uid, "cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != DeletionCancelled {
		t.Fatalf("status = %q", r.Status)
	}
}

func TestDeletionRepo_Transition_FromDeletedReturnsErrBadTransition(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	d, _ := repo.Create(ctx, uid, "", time.Now().Add(7*24*time.Hour))
	repo.Transition(ctx, d.ID, DeletionCooling, DeletionApproved, uid, "")
	repo.SetDeleted(ctx, d.ID, uid)

	_, err := repo.Transition(ctx, d.ID, DeletionDeleted, DeletionCooling, uid, "")
	if err != ErrBadTransition {
		t.Fatalf("from deleted must be ErrBadTransition, got %v", err)
	}
}
