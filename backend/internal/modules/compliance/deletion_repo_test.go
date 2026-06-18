package compliance

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	// Second Create while cooling active must return ErrDeletionExists.
	_, err = repo.Create(ctx, uid, "reason2", time.Now().Add(7*24*time.Hour))
	if err != ErrDeletionExists {
		t.Fatalf("second Create while active must return ErrDeletionExists, got %v", err)
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

func TestDeletionRepo_Create_AllowsReRequestAfterCancel(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	// First request → cooling → cancel
	d1, err := repo.Create(ctx, uid, "r1", time.Now().Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	repo.Transition(ctx, d1.ID, DeletionCooling, DeletionCancelled, uid, "changed mind")

	// Second request — must succeed (cancelled no longer blocks).
	d2, err := repo.Create(ctx, uid, "r2", time.Now().Add(7*24*time.Hour))
	if err != nil {
		t.Fatalf("re-request after cancel must succeed, got %v", err)
	}
	if d2.Status != DeletionCooling {
		t.Fatalf("status = %q, want cooling", d2.Status)
	}
}

func TestDeletionRepo_ExecuteDeletion_ScrubsPIIPreservesFinancials(t *testing.T) {
	repo, cleanup := testDeletionRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgDeletionRepo).pool
	uid := seedDelUser(t, pool)

	// Seed orders, notifications, audit_logs for this user.
	// Create seller + dataset to satisfy FK constraints. The seller account is
	// unique per run — a fixed account with ON CONFLICT DO NOTHING RETURNING would
	// yield NO row (empty uuid) on a re-run against a persistent DB, breaking the
	// order seed (the trigger that bit local re-runs).
	var sellerUUID, dsUUID string
	sellerAccount := fmt.Sprintf("ex-seller-%d@x.com", time.Now().UnixNano())
	pool.QueryRow(ctx, `INSERT INTO users (account, account_type, password_hash, role)
		VALUES ($1,'email','x','seller')
		RETURNING id::text`, sellerAccount).Scan(&sellerUUID)
	pool.QueryRow(ctx, `INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		VALUES ($1::uuid, 'ex-ds', 'text', 'commercial', 'published')
		ON CONFLICT (id) DO NOTHING
		RETURNING id::text`, sellerUUID).Scan(&dsUUID)

	if _, err := pool.Exec(ctx, `INSERT INTO orders (buyer_id, seller_id, dataset_id, license_type,
		amount_cents, platform_fee_cents, seller_amount_cents, status, product_type)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'commercial',
		100, 10, 90, 'settled', 'download')`, uid, sellerUUID, dsUUID); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	pool.Exec(ctx, `INSERT INTO notifications (user_id, kind, title, body)
		VALUES ($1, 'k', 't', 'b')`, uid)
	pool.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id)
		VALUES ($1::uuid, 'login', 'user', $1::text)`, uid)

	// Create + approve → execute
	d, _ := repo.Create(ctx, uid, "reason", time.Now().Add(-time.Hour))
	repo.Transition(ctx, d.ID, DeletionCooling, DeletionApproved, uid, "ok")

	if err := repo.ExecuteDeletion(ctx, d.ID, uid, uid); err != nil {
		t.Fatal(err)
	}

	// Assert PII scrubbed: account starts with "deleted-"
	var account string
	pool.QueryRow(ctx, `SELECT account FROM users WHERE id=$1`, uid).Scan(&account)
	if !strings.HasPrefix(account, "deleted-") {
		t.Errorf("users.account = %q, want deleted-...", account)
	}
	// kyc_status must be scrubbed to 'rejected' (only valid CHECK-compliant scrub value)
	var kyc string
	pool.QueryRow(ctx, `SELECT kyc_status FROM users WHERE id=$1`, uid).Scan(&kyc)
	if kyc != "rejected" {
		t.Errorf("kyc_status = %q, want rejected (scrubbed)", kyc)
	}

	// Assert notifications deleted.
	var n int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id=$1`, uid).Scan(&n)
	if n != 0 {
		t.Errorf("notifications count = %d, want 0", n)
	}

	// CRITICAL: orders must be preserved (accounting law).
	var orderCount int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE buyer_id=$1`, uid).Scan(&orderCount)
	if orderCount != 1 {
		t.Errorf("orders count = %d, want 1 (must preserve for accounting law)", orderCount)
	}

	// audit_logs must be preserved.
	var auditCount int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_id=$1`, uid).Scan(&auditCount)
	if auditCount != 1 {
		t.Errorf("audit_logs count = %d, want 1 (must preserve)", auditCount)
	}
}
