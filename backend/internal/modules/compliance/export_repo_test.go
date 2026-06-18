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

func testExportRepo(t *testing.T) (ExportRepository, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE data_export_jobs`)
	return NewExportRepository(pool), func() { pool.Close() }
}

func seedExportUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','buyer') RETURNING id::text`,
		"exp-"+fmt.Sprint(uniq)+"@x.com").Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestExportRepo_CreatesPendingJob(t *testing.T) {
	repo, cleanup := testExportRepo(t)
	defer cleanup()
	ctx := context.Background()
	uid := seedExportUser(t, repo.(*pgExportRepo).pool)

	j, err := repo.Create(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != ExportPending {
		t.Fatalf("status = %q, want pending", j.Status)
	}
	if j.ID == "" {
		t.Fatal("ID must not be empty")
	}
}

func TestExportRepo_PurgeByUserDeletesRowsAndReturnsKeys(t *testing.T) {
	repo, cleanup := testExportRepo(t)
	defer cleanup()
	ctx := context.Background()
	uid := seedExportUser(t, repo.(*pgExportRepo).pool)

	// A ready job (with an object key) + a pending job (key NULL).
	ready, _ := repo.Create(ctx, uid)
	if err := repo.SetReady(ctx, ready.ID, "exports/"+uid+"/z.zip", 1024, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, _ = repo.Create(ctx, uid)

	keys, err := repo.PurgeByUser(ctx, uid)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(keys) != 1 || keys[0] != "exports/"+uid+"/z.zip" {
		t.Fatalf("returned keys = %v, want the one ready object key (NULL keys skipped)", keys)
	}
	if _, err := repo.FindRecentByUser(ctx, uid); err != ErrNotFound {
		t.Fatalf("after purge FindRecentByUser = %v, want ErrNotFound", err)
	}
}

func TestExportRepo_SetReadyPopulatesObjectKeyAndExpiresAt(t *testing.T) {
	repo, cleanup := testExportRepo(t)
	defer cleanup()
	ctx := context.Background()
	uid := seedExportUser(t, repo.(*pgExportRepo).pool)

	j, _ := repo.Create(ctx, uid)
	expires := time.Now().Add(24 * time.Hour)
	if err := repo.SetReady(ctx, j.ID, "exports/u/z.zip", 1024, expires); err != nil {
		t.Fatal(err)
	}

	j2, err := repo.FindRecentByUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if j2.Status != ExportReady {
		t.Fatalf("status = %q, want ready", j2.Status)
	}
	if j2.ObjectBytes != 1024 {
		t.Fatalf("bytes = %d, want 1024", j2.ObjectBytes)
	}
}

func TestExportRepo_ExpireOldJobs_MarksReadyJobsBeyondExpiresAt(t *testing.T) {
	repo, cleanup := testExportRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgExportRepo).pool
	uid := seedExportUser(t, pool)

	j, _ := repo.Create(ctx, uid)
	repo.SetReady(ctx, j.ID, "k", 1, time.Now().Add(-time.Hour))

	if err := repo.ExpireOldJobs(ctx); err != nil {
		t.Fatal(err)
	}
	j2, err := repo.FindRecentByUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if j2.Status != ExportExpired {
		t.Fatalf("status = %q, want expired", j2.Status)
	}
}

func TestExportRepo_ExpireOldJobs_LeavesUnexpiredAlone(t *testing.T) {
	repo, cleanup := testExportRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgExportRepo).pool
	uid := seedExportUser(t, pool)

	j, _ := repo.Create(ctx, uid)
	repo.SetReady(ctx, j.ID, "k", 1, time.Now().Add(24*time.Hour))

	if err := repo.ExpireOldJobs(ctx); err != nil {
		t.Fatal(err)
	}
	j2, _ := repo.FindRecentByUser(ctx, uid)
	if j2.Status != ExportReady {
		t.Fatalf("status = %q, want ready (unexpired)", j2.Status)
	}
}
