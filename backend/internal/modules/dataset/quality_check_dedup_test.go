package dataset

import (
	"context"
	"testing"
)

// The quality retry queue is at-least-once: qualityRetryLoop can re-enqueue a
// due retry while the worker that's already processing it hasn't deleted the
// retry row yet, so processQuality runs twice for the same version. With a bare
// INSERT and no unique key, that appended a second full set of quality_checks
// rows — the buyer-facing report then showed every check twice and the quality
// ranking aggregates (count/bool_or/max) were skewed. SaveQualityCheck must be
// idempotent per (version_id, type).
func TestSaveQualityCheck_IdempotentPerVersionType(t *testing.T) {
	repo, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool

	const dsID = "00000000-0000-0000-0000-0000000c0001"
	const verID = "00000000-0000-0000-0000-0000000c00a1"
	seedDataset(t, pool, dsID)
	if _, err := pool.Exec(ctx,
		`INSERT INTO dataset_versions (id, dataset_id, version_no) VALUES ($1,$2,1) ON CONFLICT (id) DO NOTHING`,
		verID, dsID); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	pool.Exec(ctx, `DELETE FROM quality_checks WHERE version_id=$1`, verID)

	if err := repo.SaveQualityCheck(ctx, dsID, verID, "format", "pass", map[string]any{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveQualityCheck(ctx, dsID, verID, "format", "warn", map[string]any{"n": 2}); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM quality_checks WHERE version_id=$1 AND type='format'`, verID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("quality_checks rows for (version, format) = %d, want 1 (idempotent upsert)", count)
	}
	var result string
	pool.QueryRow(ctx, `SELECT result FROM quality_checks WHERE version_id=$1 AND type='format'`, verID).Scan(&result)
	if result != "warn" {
		t.Fatalf("result = %q, want warn (the latest write wins)", result)
	}
}
