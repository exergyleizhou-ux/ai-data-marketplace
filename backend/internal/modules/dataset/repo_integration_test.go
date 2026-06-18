package dataset

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// TestListPublishedQualitySummaryIntegration validates the browse-time quality
// signal against a real Postgres: ListPublished must surface the authenticity
// band/score for tabular datasets (applicable=true), omit it for text datasets
// (applicable=false), and mark both verified when no check failed.
func TestListPublishedQualitySummaryIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)

	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	var sellerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','seller') RETURNING id::text`,
		"qbench-"+uniq+"@example.com").Scan(&sellerID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	type chk struct {
		typ, res string
		rep      map[string]any
	}
	mk := func(dataType string, checks []chk) string {
		d, err := repo.Create(ctx, Dataset{SellerID: sellerID, Title: "QB " + dataType + " " + uniq, DataType: dataType, LicenseType: "commercial"})
		if err != nil {
			t.Fatalf("create %s: %v", dataType, err)
		}
		vid, err := repo.AddVersion(ctx, d.ID, "sha-"+d.ID, "sim-"+d.ID,
			FileInput{ObjectKey: "datasets/" + d.ID + "/f", SizeBytes: 10, SHA256: "sha-" + d.ID, ContentType: "text/csv"}, StatusReviewing)
		if err != nil {
			t.Fatalf("addversion: %v", err)
		}
		for _, c := range checks {
			if err := repo.SaveQualityCheck(ctx, d.ID, vid, c.typ, c.res, c.rep); err != nil {
				t.Fatalf("savecheck: %v", err)
			}
		}
		if err := repo.SetStatus(ctx, d.ID, StatusPublished); err != nil {
			t.Fatalf("setstatus: %v", err)
		}
		return d.ID
	}

	tabID := mk("structured", []chk{
		{"authenticity", "pass", map[string]any{"applicable": true, "band": "clean", "score": 92}},
		{"pii_redaction", "pass", map[string]any{"verified": true}},
	})
	txtID := mk("text", []chk{
		{"authenticity", "pass", map[string]any{"applicable": false, "band": "clean", "score": 100}},
		{"pii_redaction", "pass", map[string]any{"verified": true}},
	})

	list, err := repo.ListPublished(ctx, ListFilter{Limit: 100})
	if err != nil {
		t.Fatalf("listpublished: %v", err)
	}
	got := map[string]Dataset{}
	for _, d := range list {
		got[d.ID] = d
	}

	tab, ok := got[tabID]
	if !ok {
		t.Fatal("tabular dataset missing from published list")
	}
	if tab.AuthenticityBand != "clean" {
		t.Errorf("tabular band = %q, want clean", tab.AuthenticityBand)
	}
	if tab.AuthenticityScore == nil || *tab.AuthenticityScore != 92 {
		t.Errorf("tabular score = %v, want 92", tab.AuthenticityScore)
	}
	if tab.QualityVerified == nil || !*tab.QualityVerified {
		t.Errorf("tabular should be quality_verified, got %v", tab.QualityVerified)
	}

	txt, ok := got[txtID]
	if !ok {
		t.Fatal("text dataset missing from published list")
	}
	if txt.AuthenticityBand != "" || txt.AuthenticityScore != nil {
		t.Errorf("text dataset must not surface an authenticity band/score (applicable=false), got band=%q score=%v", txt.AuthenticityBand, txt.AuthenticityScore)
	}
	if txt.QualityVerified == nil || !*txt.QualityVerified {
		t.Errorf("text dataset should still be quality_verified (no fails), got %v", txt.QualityVerified)
	}
}

// TestCroissantMetadataIntegration validates the full Croissant path against a
// real Postgres: CurrentVersionMeta's 3-table join must surface the file's
// content type / size / sha256 into the JSON-LD distribution.
func TestCroissantMetadataIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)

	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	var sellerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','seller') RETURNING id::text`,
		"cr-"+uniq+"@example.com").Scan(&sellerID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	d, err := repo.Create(ctx, Dataset{SellerID: sellerID, Title: "CR " + uniq, DataType: "structured", LicenseType: "commercial"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	vid, err := repo.AddVersion(ctx, d.ID, "feedface", "sim",
		FileInput{ObjectKey: "datasets/" + d.ID + "/data.csv", SizeBytes: 2048, SHA256: "feedface", ContentType: "text/csv"}, StatusReviewing)
	if err != nil {
		t.Fatalf("addversion: %v", err)
	}
	if err := repo.SaveQualityCheck(ctx, d.ID, vid, "authenticity", "pass", map[string]any{"applicable": true, "band": "clean", "score": 90}); err != nil {
		t.Fatalf("savecheck: %v", err)
	}

	// Croissant is a public, published-dataset feature; publish before reading.
	if err := repo.SetStatus(ctx, d.ID, StatusPublished); err != nil {
		t.Fatalf("publish: %v", err)
	}

	svc := NewService(repo, fakeIdentity{status: map[string]string{}}, nil)
	doc, err := svc.CroissantMetadata(ctx, d.ID, "https://oasis.example")
	if err != nil {
		t.Fatalf("croissant: %v", err)
	}
	if doc["conformsTo"] != "http://mlcommons.org/croissant/1.0" {
		t.Errorf("conformsTo = %v", doc["conformsTo"])
	}
	dist, ok := doc["distribution"].([]any)
	if !ok || len(dist) != 1 {
		t.Fatalf("distribution = %v", doc["distribution"])
	}
	fo := dist[0].(map[string]any)
	if fo["encodingFormat"] != "text/csv" {
		t.Errorf("encodingFormat = %v (3-table join lost content_type)", fo["encodingFormat"])
	}
	if fo["sha256"] != "feedface" {
		t.Errorf("sha256 = %v", fo["sha256"])
	}
	if fo["contentSize"] != "2048 B" {
		t.Errorf("contentSize = %v", fo["contentSize"])
	}
}

// TestDatasheetRoundTripIntegration validates migration 000008 + the JSONB scan
// against a real Postgres: a datasheet set via SetDatasheet must read back via
// GetByID, and a nil datasheet must clear it.
func TestDatasheetRoundTripIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)

	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	var sellerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','seller') RETURNING id::text`,
		"sheet-"+uniq+"@example.com").Scan(&sellerID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d, err := repo.Create(ctx, Dataset{SellerID: sellerID, Title: "Sheet " + uniq, DataType: "structured", LicenseType: "commercial"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := repo.SetDatasheet(ctx, d.ID, &Datasheet{IntendedUses: "Pretraining", Limitations: "finance-skewed", Languages: []string{"zh", "en"}}); err != nil {
		t.Fatalf("set datasheet: %v", err)
	}
	got, err := repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("getbyid: %v", err)
	}
	if got.Datasheet == nil || got.Datasheet.IntendedUses != "Pretraining" || len(got.Datasheet.Languages) != 2 {
		t.Fatalf("datasheet round-trip lost data: %+v", got.Datasheet)
	}

	if _, err := repo.SetDatasheet(ctx, d.ID, nil); err != nil {
		t.Fatalf("clear datasheet: %v", err)
	}
	got2, _ := repo.GetByID(ctx, d.ID)
	if got2.Datasheet != nil {
		t.Errorf("nil datasheet should clear, got %+v", got2.Datasheet)
	}
}

// TestSortByQualityIntegration validates that sort=quality orders a clean,
// high-score tabular dataset ahead of a low-score one against real Postgres
// (exercises the LATERAL join + ORDER BY).
func TestSortByQualityIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)

	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	var sellerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (account, account_type, password_hash, role)
		 VALUES ($1,'email','x','seller') RETURNING id::text`,
		"qsort-"+uniq+"@example.com").Scan(&sellerID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mk := func(title string, score int) string {
		d, err := repo.Create(ctx, Dataset{SellerID: sellerID, Title: title + " " + uniq, DataType: "structured", LicenseType: "commercial"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		vid, err := repo.AddVersion(ctx, d.ID, "sha-"+d.ID, "sim", FileInput{ObjectKey: "datasets/" + d.ID + "/d.csv", SizeBytes: 1, SHA256: "s", ContentType: "text/csv"}, StatusReviewing)
		if err != nil {
			t.Fatalf("addversion: %v", err)
		}
		band := "clean"
		if score < 60 {
			band = "suspect"
		}
		if err := repo.SaveQualityCheck(ctx, d.ID, vid, "authenticity", "pass", map[string]any{"applicable": true, "band": band, "score": score}); err != nil {
			t.Fatalf("savecheck: %v", err)
		}
		if err := repo.SetStatus(ctx, d.ID, StatusPublished); err != nil {
			t.Fatalf("setstatus: %v", err)
		}
		return d.ID
	}
	hi := mk("HiQ", 95)
	lo := mk("LoQ", 30)

	list, err := repo.ListPublished(ctx, ListFilter{Sort: "quality", Limit: 100, Keyword: uniq})
	if err != nil {
		t.Fatalf("listpublished: %v", err)
	}
	var hiPos, loPos = -1, -1
	for i, d := range list {
		if d.ID == hi {
			hiPos = i
		}
		if d.ID == lo {
			loPos = i
		}
	}
	if hiPos < 0 || loPos < 0 {
		t.Fatalf("both datasets must be present (hi=%d lo=%d)", hiPos, loPos)
	}
	if hiPos > loPos {
		t.Errorf("sort=quality: high-score (pos %d) must come before low-score (pos %d)", hiPos, loPos)
	}
}
