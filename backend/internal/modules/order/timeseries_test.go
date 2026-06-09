package order

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// testPool opens an ephemeral PG and runs the production migrations.
// Caller must clean up its OWN test data (TRUNCATE the rows it inserted) — never
// DROP TABLE.  Other packages share this PG via -p 1.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping PG integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_, _ = pool.Exec(context.Background(), `
		TRUNCATE TABLE settlement_outbox CASCADE;
		TRUNCATE TABLE orders CASCADE;
	`)
	return pool
}

// uniqSuffix returns a hex-encoded random 8-byte string, guaranteed unique
// across rapid successive calls.
func uniqSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// seedUser inserts a real user row with a UUID id and returns the UUID string.
func seedUser(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	suf := uniqSuffix()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x',$2,'verified') RETURNING id::text`,
		"ts-"+role+"-"+suf+"@x.com", role).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedDataset inserts a real datasets row with the given seller_id and a unique
// title.  Returns the dataset UUID string.
func seedDataset(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		 VALUES ($1, $2, 'text', 'commercial', 'published') RETURNING id::text`,
		sellerID, "ts-ds-"+uniqSuffix()).Scan(&id); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	return id
}

// uniqOrderID returns a random UUID string for a test order.
func uniqOrderID(t *testing.T) string {
	t.Helper()
	return "00000000-0000-0000-0000-" + uniqSuffix()[:12]
}

func insertOrder(t *testing.T, pool *pgxpool.Pool, o Order) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO orders (id, buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type, created_at)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, NULLIF($5,'')::uuid, $6, $7, $8, $9, $10, $11, $12::timestamptz)`,
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

	buyer := seedUser(t, pool, "buyer")
	seller := seedUser(t, pool, "seller")
	dsA := seedDataset(t, pool, seller)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

	// Insert 2 orders yesterday, 1 settled.
	insertOrder(t, pool, Order{
		ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsA,
		VersionID: "", LicenseType: "commercial",
		AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: yesterdayStr + "T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsA,
		VersionID: "", LicenseType: "commercial",
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

	seller := seedUser(t, pool, "seller")
	buyer := seedUser(t, pool, "buyer")
	dsA := seedDataset(t, pool, seller)
	dsB := seedDataset(t, pool, seller)

	insertOrder(t, pool, Order{
		ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsA,
		VersionID: "", LicenseType: "commercial",
		AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-01T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsA,
		VersionID: "", LicenseType: "commercial",
		AmountCents: 50000, PlatformFeeCents: 5000, SellerAmountCents: 45000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-02T10:00:00Z",
	})
	insertOrder(t, pool, Order{
		ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsB,
		VersionID: "", LicenseType: "commercial",
		AmountCents: 20000, PlatformFeeCents: 2000, SellerAmountCents: 18000,
		Status: StatusSettled, ProductType: ProductDownload,
		CreatedAt: "2026-06-03T10:00:00Z",
	})

	items, err := repo.SellerEarningsByDataset(ctx, seller)
	if err != nil {
		t.Fatalf("by-dataset: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// Should be sorted by settled_cents DESC: dsA (135000) first, dsB (18000) second.
	if items[0].SettledCents != 135000 {
		t.Errorf("dsA settled = %d, want 135000", items[0].SettledCents)
	}
	if items[0].TotalOrders != 2 {
		t.Errorf("dsA orders = %d, want 2", items[0].TotalOrders)
	}
	if items[1].SettledCents != 18000 {
		t.Errorf("dsB settled = %d, want 18000", items[1].SettledCents)
	}
	if items[1].TotalOrders != 1 {
		t.Errorf("dsB orders = %d, want 1", items[1].TotalOrders)
	}
}
