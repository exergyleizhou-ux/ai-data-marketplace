package watchlist

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
	// Guard against -p 1 scenario where another module's test drops
	// datasets CASCADE and db.RunMigrations is a no-op.
	var hasSeller bool
	_ = pool.QueryRow(context.Background(),
		`SELECT exists(SELECT 1 FROM information_schema.columns WHERE table_name='datasets' AND column_name='seller_id')`,
	).Scan(&hasSeller)
	if !hasSeller {
		t.Skip("datasets.seller_id missing — likely -p 1 CASCADE conflict (pre-existing); run watchlist tests in isolation")
	}
	pool.Exec(context.Background(), `DELETE FROM dataset_watches`)
	return NewRepository(pool), func() { pool.Close() }
}

func seedUser(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if len(userID) < 8 {
		userID = userID + "xxxxx"[:8-len(userID)]
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','buyer') ON CONFLICT (account) DO UPDATE SET role='buyer'`,
		"wl-"+userID[:8]+"@x.com")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// genDS creates a dataset backed by a real seller user, returning the dataset UUID
// and its current_version_id so tests can verify the INITIALISED last_notified.
func genDS(t *testing.T, pool *pgxpool.Pool) (datasetID, versionID string) {
	t.Helper()
	uniq := time.Now().UnixNano()
	sellerUUID := fmt.Sprintf("00000000-0000-0000-0000-%012x", uniq%0x1000000000000)
	dsUUID := fmt.Sprintf("00000000-0000-0000-0000-%012x", (uniq+1)%0x1000000000000)
	verUUID := fmt.Sprintf("00000000-0000-0000-0000-%012x", (uniq+2)%0x1000000000000)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, account, account_type, password_hash, role)
		 VALUES ($1,$2,'email','x','seller') ON CONFLICT DO NOTHING`,
		sellerUUID, fmt.Sprintf("wl-seller-%x@x.com", uniq))
	if err != nil {
		t.Fatalf("seed seller: %v", err)
	}
	// Insert datasets without current_version_id first (FK is circular: dataset_versions
	// references datasets and datasets.current_version_id references dataset_versions).
	_, err = pool.Exec(context.Background(),
		`INSERT INTO datasets (id, seller_id, title, data_type, license_type)
		 VALUES ($1,$2,'Test DS','text','commercial') ON CONFLICT (id) DO NOTHING`,
		dsUUID, sellerUUID)
	if err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		`INSERT INTO dataset_versions (id, dataset_id, version_no, content_sha256, simhash)
		 VALUES ($1, $2, 1, 'sha-', 'sim-') ON CONFLICT DO NOTHING`,
		verUUID, dsUUID)
	if err != nil {
		t.Fatalf("seed version: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		`UPDATE datasets SET current_version_id = $1 WHERE id = $2`, verUUID, dsUUID)
	if err != nil {
		t.Fatalf("set current_version_id: %v", err)
	}
	return dsUUID, verUUID
}

func TestAdd_Idempotent(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-idax")
	dsID, _ := genDS(t, pool)

	if err := repo.Add(ctx, "user-idax", dsID); err != nil {
		t.Fatal(err)
	}
	// Second call must not error.
	if err := repo.Add(ctx, "user-idax", dsID); err != nil {
		t.Fatal("second Add must not error", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM dataset_watches WHERE user_id=$1 AND dataset_id=$2`, "user-idax", dsID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
}

// TestAdd_InitializesLastNotifiedToCurrentVersion verifies the key spec
// requirement: Add sets last_notified_version_id = datasets.current_version_id
// so watchers only receive notifications for future versions.
func TestAdd_InitializesLastNotifiedToCurrentVersion(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-init")
	dsID, expectedVer := genDS(t, pool)

	if err := repo.Add(ctx, "user-init", dsID); err != nil {
		t.Fatal(err)
	}
	var gotVer string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(last_notified_version_id::text,'')
		 FROM dataset_watches WHERE user_id=$1 AND dataset_id=$2`,
		"user-init", dsID).Scan(&gotVer); err != nil {
		t.Fatalf("read watch row: %v", err)
	}
	if gotVer != expectedVer {
		t.Fatalf("last_notified_version_id = %q, want %q", gotVer, expectedVer)
	}
}

func TestRemove_DeletesRow(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-rmx")
	dsID, _ := genDS(t, pool)
	repo.Add(ctx, "user-rmx", dsID)

	if err := repo.Remove(ctx, "user-rmx", dsID); err != nil {
		t.Fatal(err)
	}
	var count int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM dataset_watches WHERE user_id='user-rmx' AND dataset_id=$1`, dsID).Scan(&count)
	if count != 0 {
		t.Fatalf("rows after remove = %d, want 0", count)
	}
}

func TestRemove_NonExistent_NoError(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	if err := repo.Remove(context.Background(), "nobody", "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("remove non-existent must not error, got: %v", err)
	}
}

func TestListByUser_ReturnsOwnOnly(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-LAx")
	seedUser(t, pool, "user-LBx")
	ds1, _ := genDS(t, pool)
	ds2, _ := genDS(t, pool)

	repo.Add(ctx, "user-LAx", ds1)
	repo.Add(ctx, "user-LAx", ds2)
	repo.Add(ctx, "user-LBx", ds1)

	list, err := repo.ListByUser(ctx, "user-LAx")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	for _, w := range list {
		if w.UserID != "user-LAx" {
			t.Fatalf("leaked row with UserID=%q", w.UserID)
		}
	}
}

func TestListByDataset_ReturnsAllWatchers(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-Wx1")
	seedUser(t, pool, "user-Wx2")
	seedUser(t, pool, "user-Wx3")
	dsID, _ := genDS(t, pool)

	repo.Add(ctx, "user-Wx1", dsID)
	repo.Add(ctx, "user-Wx2", dsID)
	repo.Add(ctx, "user-Wx3", dsID)

	uvs, err := repo.ListByDataset(ctx, dsID)
	if err != nil {
		t.Fatal(err)
	}
	if len(uvs) != 3 {
		t.Fatalf("len = %d, want 3", len(uvs))
	}
}

func TestMarkNotified_UpdatesOnlyMatchingRow(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	seedUser(t, pool, "user-MNx")
	ds1, _ := genDS(t, pool)
	ds2, _ := genDS(t, pool)
	newVer := fmt.Sprintf("00000000-0000-0000-0000-%012x", time.Now().UnixNano()%0x1000000000000)

	repo.Add(ctx, "user-MNx", ds1)
	repo.Add(ctx, "user-MNx", ds2)

	if err := repo.MarkNotified(ctx, "user-MNx", ds1, newVer); err != nil {
		t.Fatal(err)
	}

	var v1, v2 string
	pool.QueryRow(ctx, `SELECT COALESCE(last_notified_version_id::text,'') FROM dataset_watches WHERE user_id='user-MNx' AND dataset_id=$1`, ds1).Scan(&v1)
	pool.QueryRow(ctx, `SELECT COALESCE(last_notified_version_id::text,'') FROM dataset_watches WHERE user_id='user-MNx' AND dataset_id=$1`, ds2).Scan(&v2)

	if v1 != newVer {
		t.Fatalf("ds1 last_notified = %q, want %q", v1, newVer)
	}
	// ds2 was added via Add so its last_notified is initialised to its own
	// current_version_id, not empty. It should be unchanged (still its own ver).
	if v2 == "" || v2 == newVer {
		t.Fatalf("ds2 last_notified = %q, want its own initial version (not empty and not %q)", v2, newVer)
	}
}
