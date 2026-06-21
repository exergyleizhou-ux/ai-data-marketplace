// Command seedpublic publishes a curated set of real, public research datasets
// into a marketplace instance — the showcase supply for the verified data
// marketplace's research beachhead.
//
// It is deliberately honest: each dataset's bytes come from the cited public
// source, the verification rows are produced by the platform's OWN quality
// library (the same code path a real upload runs), the content fingerprint is
// computed by the storage driver, and the integrity certificate is the
// deterministic VO- code over that fingerprint — so anyone can re-download,
// re-hash, and re-verify. Nothing here is hand-faked.
//
// Usage:
//
//	DATABASE_URL=postgres://... STORAGE_DIR=/path/to/storage \
//	  go run ./cmd/seedpublic
//
// It is idempotent: a dataset whose title already exists for the seed seller is
// skipped, so re-running tops up missing datasets without duplicating.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/dataset"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/quality"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

const (
	seedSellerAccount = "research-desk@oasis.demo"
	contentType       = "text/csv"
)

func main() {
	dsn := mustEnv("DATABASE_URL")
	storageDir := mustEnv("STORAGE_DIR")

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	store, err := storage.NewLocal(storageDir)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	repo := dataset.NewRepository(pool)

	sellerID, err := ensureSeller(ctx, pool)
	if err != nil {
		log.Fatalf("ensure seed seller: %v", err)
	}
	log.Printf("seed seller %s = %s", seedSellerAccount, sellerID)

	client := &http.Client{Timeout: 60 * time.Second}
	var created, skipped, failed int
	fmt.Printf("\n%-22s %-26s %-9s %-6s %s\n", "KEY", "DOMAIN", "BAND", "SCORE", "DATASET_ID")
	fmt.Println("-------------------------------------------------------------------------------------------")
	for _, item := range seedDatasets {
		res, err := seedOne(ctx, client, pool, repo, store, sellerID, item)
		switch {
		case err != nil:
			failed++
			log.Printf("FAIL %-20s %v", item.Key, err)
		case res.skipped:
			skipped++
			fmt.Printf("%-22s %-26s %-9s %-6s %s (skip: exists)\n", item.Key, item.Domain, "-", "-", res.datasetID)
		default:
			created++
			fmt.Printf("%-22s %-26s %-9s %-6d %s\n", item.Key, item.Domain, res.band, res.score, res.datasetID)
		}
	}
	fmt.Printf("\nseeded=%d skipped=%d failed=%d\n", created, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

type seedResult struct {
	datasetID string
	band      string
	score     int
	skipped   bool
}

// seedOne fetches, normalizes, stores, and verifies one dataset end-to-end via
// the platform's real storage + quality + repo code paths.
func seedOne(ctx context.Context, client *http.Client, pool *pgxpool.Pool, repo dataset.Repository, store *storage.Local, sellerID string, item seedItem) (seedResult, error) {
	// Idempotency: skip if this seed seller already has a dataset by this title.
	var existingID string
	err := pool.QueryRow(ctx, `SELECT id::text FROM datasets WHERE seller_id=$1 AND title=$2 LIMIT 1`, sellerID, item.TitleZH).Scan(&existingID)
	if err == nil {
		return seedResult{datasetID: existingID, skipped: true}, nil
	}

	raw, err := fetch(ctx, client, item.SourceURL)
	if err != nil {
		return seedResult{}, fmt.Errorf("fetch: %w", err)
	}
	csvBytes, err := normalizeToCSV(raw, item.Format)
	if err != nil {
		return seedResult{}, fmt.Errorf("normalize: %w", err)
	}
	csvBytes = prependHeader(csvBytes, item.Header)

	// Run the platform's own quality checks FIRST. They are pure functions over
	// the bytes, so computing them before any DB/storage write guarantees a
	// genuine FAIL leaves nothing published (no half-seeded, unverified dataset).
	fmtChk := quality.Format(csvBytes, contentType)
	piiChk := quality.PII(csvBytes, false)
	statsChk, sample := quality.Stats(csvBytes)
	authChk := quality.Authenticity(csvBytes, contentType)
	checks := []quality.Check{fmtChk, piiChk, statsChk, authChk}
	for _, c := range checks {
		if c.Result == quality.ResultFail {
			return seedResult{}, fmt.Errorf("quality %s returned FAIL: %v", c.Type, c.Report)
		}
	}

	price := item.PriceCents
	ds, err := repo.Create(ctx, dataset.Dataset{
		SellerID:            sellerID,
		Title:               item.TitleZH,
		Description:         description(item),
		DataType:            item.DataType,
		Domain:              item.Domain,
		LicenseType:         item.LicenseType,
		SuggestedPriceCents: &price,
	})
	if err != nil {
		return seedResult{}, fmt.Errorf("create dataset: %w", err)
	}

	// Store the bytes via the real storage driver so the file is genuinely
	// downloadable and its SHA-256 is computed by the platform (not by us).
	key := "datasets/" + ds.ID + "/data.csv"
	uploadID, err := store.InitMultipart(ctx, key)
	if err != nil {
		return seedResult{}, fmt.Errorf("init upload: %w", err)
	}
	if _, err := store.PutPart(ctx, uploadID, 1, bytes.NewReader(csvBytes)); err != nil {
		return seedResult{}, fmt.Errorf("put part: %w", err)
	}
	obj, err := store.CompleteMultipart(ctx, uploadID)
	if err != nil {
		return seedResult{}, fmt.Errorf("complete upload: %w", err)
	}

	versionID, err := repo.AddVersion(ctx, ds.ID, obj.SHA256, "", dataset.FileInput{
		ObjectKey:   obj.Key,
		SizeBytes:   obj.Size,
		SHA256:      obj.SHA256,
		ContentType: contentType,
	}, dataset.StatusPublished)
	if err != nil {
		return seedResult{}, fmt.Errorf("add version: %w", err)
	}

	// Persist the verification rows — exactly what a real upload produces.
	if err := repo.SetVersionSimhash(ctx, versionID, quality.SimHash(csvBytes)); err != nil {
		return seedResult{}, fmt.Errorf("simhash: %w", err)
	}
	for _, c := range checks {
		if err := repo.SaveQualityCheck(ctx, ds.ID, versionID, c.Type, c.Result, c.Report); err != nil {
			return seedResult{}, fmt.Errorf("save quality check %s: %w", c.Type, err)
		}
	}
	if err := repo.SetSampleCount(ctx, ds.ID, sample); err != nil {
		return seedResult{}, fmt.Errorf("sample count: %w", err)
	}

	band, _ := authChk.Report["band"].(string)
	score := reportInt(authChk.Report["score"])
	return seedResult{datasetID: ds.ID, band: band, score: score}, nil
}

func description(item seedItem) string {
	return item.DescZH + "\n\n" + item.DescEN +
		"\n\n来源 / Source: " + item.Citation +
		"\n许可 / License: " + item.LicenseNote +
		"\n" + item.SourceURL
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oasis-seedpublic/1.0 (research dataset seeder)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ensureSeller upserts the dedicated seed seller account and returns its id. The
// account is non-loginnable (placeholder hash); it exists only to own the
// platform's showcase datasets.
func ensureSeller(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (account, account_type, password_hash, role, kyc_status, status)
		VALUES ($1, 'email', 'disabled-seed-account', 'seller', 'verified', 'active')
		ON CONFLICT (account) DO UPDATE SET role='seller', kyc_status='verified', updated_at=now()
		RETURNING id::text`, seedSellerAccount).Scan(&id)
	return id, err
}

func reportInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
