package order

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testRecPool(t *testing.T) (Repository, func()) {
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
	pool.Exec(context.Background(),
		`TRUNCATE TABLE settlement_outbox CASCADE; TRUNCATE TABLE orders CASCADE`)
	return NewRepository(pool), func() { pool.Close() }
}

func recUniqSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func recSeedUser(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x',$2,'verified') RETURNING id::text`,
		"rec-"+role+"-"+recUniqSuffix()+"@x.com", role).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func recSeedDataset(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		 VALUES ($1, 'rec-ds', 'text', 'commercial', 'published') RETURNING id::text`,
		sellerID).Scan(&id); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	return id
}

func recInsertOrder(t *testing.T, pool *pgxpool.Pool, buyerID, sellerID, datasetID string, amount, fee, sellerAmt int64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO orders (buyer_id, seller_id, dataset_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type)
		 VALUES ($1::uuid,$2::uuid,$3::uuid,'commercial',$4,$5,$6,$7,'download')`,
		buyerID, sellerID, datasetID, amount, fee, sellerAmt, status)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
}

func TestPgRepo_AdminReconciliation_EmptyDatabase(t *testing.T) {
	repo, cleanup := testRecPool(t)
	defer cleanup()
	rec, err := repo.AdminReconciliation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rec.TotalOrders != 0 || rec.SettledGMV != 0 || rec.PendingOrders != 0 {
		t.Fatalf("empty DB must produce zeros: %+v", rec)
	}
}

func TestPgRepo_AdminReconciliation_AggregatesAcrossStatuses(t *testing.T) {
	repo, cleanup := testRecPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	seller := recSeedUser(t, pool, "seller")
	buyer := recSeedUser(t, pool, "buyer")
	ds1 := recSeedDataset(t, pool, seller)
	ds2 := recSeedDataset(t, pool, seller)
	ds3 := recSeedDataset(t, pool, seller)
	ds4 := recSeedDataset(t, pool, seller)

	recInsertOrder(t, pool, buyer, seller, ds1, 10000, 1000, 9000, StatusSettled)
	recInsertOrder(t, pool, buyer, seller, ds2, 20000, 2000, 18000, StatusRefunded)
	recInsertOrder(t, pool, buyer, seller, ds3, 30000, 3000, 27000, StatusPaid)
	recInsertOrder(t, pool, buyer, seller, ds4, 40000, 4000, 36000, StatusDisputed)

	rec, err := repo.AdminReconciliation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rec.TotalOrders != 4 {
		t.Fatalf("TotalOrders = %d, want 4", rec.TotalOrders)
	}
	if rec.SettledOrders != 1 {
		t.Fatalf("SettledOrders = %d, want 1", rec.SettledOrders)
	}
	if rec.RefundedOrders != 1 {
		t.Fatalf("RefundedOrders = %d, want 1", rec.RefundedOrders)
	}
	if rec.DisputedOrders != 1 {
		t.Fatalf("DisputedOrders = %d, want 1", rec.DisputedOrders)
	}
	if rec.PendingOrders != 1 {
		t.Fatalf("PendingOrders = %d, want 1", rec.PendingOrders)
	}
}

func TestPgRepo_AdminReconciliation_PlatformFeesOnlyCountSettled(t *testing.T) {
	repo, cleanup := testRecPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	seller := recSeedUser(t, pool, "seller")
	buyer := recSeedUser(t, pool, "buyer")
	dsP := recSeedDataset(t, pool, seller)
	dsS := recSeedDataset(t, pool, seller)

	recInsertOrder(t, pool, buyer, seller, dsP, 1000, 500, 500, StatusPaid)
	recInsertOrder(t, pool, buyer, seller, dsS, 2000, 200, 1800, StatusSettled)

	rec, err := repo.AdminReconciliation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rec.PlatformFees != 200 {
		t.Fatalf("PlatformFees = %d, want 200 (only settled fees)", rec.PlatformFees)
	}
}

func TestPgRepo_AdminReconciliation_RefundedAmountSumsCorrectly(t *testing.T) {
	repo, cleanup := testRecPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	seller := recSeedUser(t, pool, "seller")
	buyer := recSeedUser(t, pool, "buyer")
	dsID := recSeedDataset(t, pool, seller)

	recInsertOrder(t, pool, buyer, seller, dsID, 10000, 1000, 9000, StatusRefunded)
	recInsertOrder(t, pool, buyer, seller, dsID, 5000, 500, 4500, StatusRefunded)

	rec, err := repo.AdminReconciliation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rec.RefundedAmount != 15000 {
		t.Fatalf("RefundedAmount = %d, want 15000", rec.RefundedAmount)
	}
}

func TestPgRepo_AdminReconciliation_FailedSettlements_ToleratesAbsentTable(t *testing.T) {
	repo, cleanup := testRecPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	seller := recSeedUser(t, pool, "seller")
	buyer := recSeedUser(t, pool, "buyer")
	dsID := recSeedDataset(t, pool, seller)
	recInsertOrder(t, pool, buyer, seller, dsID, 1000, 100, 900, StatusSettled)

	// TRUNCATE settlement_outbox to simulate it being absent/test-only.
	pool.Exec(ctx, `TRUNCATE TABLE settlement_outbox`)

	rec, err := repo.AdminReconciliation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// FailedSettlements must be 0 when table is empty, not crash.
	if rec.FailedSettlements != 0 {
		t.Fatalf("FailedSettlements = %d, want 0 (absent table must not crash)", rec.FailedSettlements)
	}
}
