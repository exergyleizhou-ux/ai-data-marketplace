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
