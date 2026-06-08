package order

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool opens the ephemeral PG. The caller must have DATABASE_URL set, or
// the test is skipped (safe for CI without a DB).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping PG integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Drop existing tables so schema changes (e.g. dataset_id type) take effect.
	pool.Exec(context.Background(), `DROP TABLE IF EXISTS settlement_outbox, orders, datasets CASCADE`)
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS orders (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			buyer_id TEXT NOT NULL,
			seller_id TEXT NOT NULL,
			dataset_id UUID NOT NULL,
			version_id UUID,
			license_type TEXT NOT NULL,
			amount_cents BIGINT NOT NULL,
			platform_fee_cents BIGINT NOT NULL,
			seller_amount_cents BIGINT NOT NULL,
			status TEXT NOT NULL,
			product_type TEXT NOT NULL DEFAULT 'download',
			auto_confirm_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("create orders table: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS settlement_outbox (
			order_id UUID PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INT NOT NULL DEFAULT 0,
			last_error TEXT,
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("create settlement_outbox table: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS datasets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("create datasets table: %v", err)
	}
	return pool
}

func insertOrder(t *testing.T, pool *pgxpool.Pool, o Order) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO orders (id, buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type, created_at)
		SELECT $1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12::timestamptz`,
		o.ID, o.BuyerID, o.SellerID, o.DatasetID, o.VersionID, o.LicenseType,
		o.AmountCents, o.PlatformFeeCents, o.SellerAmountCents, o.Status, o.ProductType, o.CreatedAt)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
}

// §7.6 Test 1: Zero-day fill + accurate GMV on target day.
func TestAdminReconciliationTimeseries_FillsZeroDays(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	repo := &pgRepo{pool: pool}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

	// Insert 2 orders yesterday, 1 settled.
	insertOrder(t, pool, Order{
		ID: "00000000-0000-0000-0000-000000000001", BuyerID: "b1", SellerID: "s1", DatasetID: "c0000000-0000-0000-0000-000000000001",
		VersionID: "00000000-0000-0000-0000-000000000000", LicenseType: "commercial",
		AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: yesterdayStr + "T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: "00000000-0000-0000-0000-000000000002", BuyerID: "b2", SellerID: "s1", DatasetID: "c0000000-0000-0000-0000-000000000002",
		VersionID: "00000000-0000-0000-0000-000000000000", LicenseType: "commercial",
		AmountCents: 50000, PlatformFeeCents: 5000, SellerAmountCents: 45000,
		Status: StatusRefunded, ProductType: ProductDownload,
		CreatedAt: yesterdayStr + "T12:00:00Z",
	})

	pts, err := repo.AdminReconciliationTimeseries(ctx, 30)
	if err != nil {
		t.Fatalf("timeseries: %v", err)
	}
	if len(pts) != 30 {
		t.Fatalf("got %d points, want 30", len(pts))
	}
	// Find yesterday's point.
	var ypt *ReconciliationPoint
	for i := range pts {
		if pts[i].Date == yesterdayStr {
			ypt = &pts[i]
			break
		}
	}
	if ypt == nil {
		t.Fatalf("yesterday (%s) not found in points", yesterdayStr)
	}
	if ypt.GMVCents != 150000 {
		t.Errorf("gmv = %d, want 150000", ypt.GMVCents)
	}
	if ypt.SettledGMVCents != 100000 {
		t.Errorf("settled_gmv = %d, want 100000", ypt.SettledGMVCents)
	}
	if ypt.PlatformFeesCents != 10000 {
		t.Errorf("platform_fees = %d, want 10000", ypt.PlatformFeesCents)
	}
	if ypt.Orders != 2 {
		t.Errorf("orders = %d, want 2", ypt.Orders)
	}
	if ypt.RefundedOrders != 1 {
		t.Errorf("refunded = %d, want 1", ypt.RefundedOrders)
	}
	// All other days should be zero.
	otherNonZero := false
	for i := range pts {
		if pts[i].Date != yesterdayStr && pts[i].GMVCents != 0 {
			otherNonZero = true
		}
	}
	if otherNonZero {
		t.Error("non-yesterday days should be zero-filled")
	}
}

// §7.6 Test 3: Per-dataset aggregation.
func TestSellerEarningsByDataset_AggregatesPerDataset(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	repo := &pgRepo{pool: pool}

	insertOrder(t, pool, Order{
		ID: "10000000-0000-0000-0000-000000000001", BuyerID: "b1", SellerID: "seller-agg", DatasetID: "a0000000-0000-0000-0000-000000000001",
		VersionID: "00000000-0000-0000-0000-000000000000", LicenseType: "commercial",
		AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-01T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: "10000000-0000-0000-0000-000000000002", BuyerID: "b2", SellerID: "seller-agg", DatasetID: "a0000000-0000-0000-0000-000000000001",
		VersionID: "00000000-0000-0000-0000-000000000000", LicenseType: "commercial",
		AmountCents: 50000, PlatformFeeCents: 5000, SellerAmountCents: 45000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-02T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: "10000000-0000-0000-0000-000000000003", BuyerID: "b3", SellerID: "seller-agg", DatasetID: "b0000000-0000-0000-0000-000000000001",
		VersionID: "00000000-0000-0000-0000-000000000000", LicenseType: "commercial",
		AmountCents: 20000, PlatformFeeCents: 2000, SellerAmountCents: 18000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-03T10:00:00Z",
	})
	// Insert matching dataset rows.
	pool.Exec(context.Background(), `INSERT INTO datasets (id, title) VALUES ('a0000000-0000-0000-0000-000000000001','数据集A'),('b0000000-0000-0000-0000-000000000001','数据集B')`)

	items, err := repo.SellerEarningsByDataset(ctx, "seller-agg")
	if err != nil {
		t.Fatalf("by-dataset: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// Should be sorted by settled_cents DESC: dset-a first.
	if items[0].DatasetID != "a0000000-0000-0000-0000-000000000001" {
		t.Errorf("first item = %s, want a0000000-...", items[0].DatasetID)
	}
	if items[0].SettledCents != 135000 {
		t.Errorf("dset-a settled = %d, want 135000", items[0].SettledCents)
	}
	if items[0].TotalOrders != 2 {
		t.Errorf("dset-a orders = %d, want 2", items[0].TotalOrders)
	}
	if items[1].DatasetID != "b0000000-0000-0000-0000-000000000001" {
		t.Errorf("second item = %s, want b0000000-...", items[1].DatasetID)
	}
}
