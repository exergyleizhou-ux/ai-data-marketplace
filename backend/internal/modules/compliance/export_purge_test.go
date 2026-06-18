package compliance

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// purgeFakeRepo is a minimal ExportRepository for the PurgeUser glue test.
type purgeFakeRepo struct{ keys []string }

func (r *purgeFakeRepo) Create(context.Context, string) (ExportJob, error) {
	return ExportJob{}, nil
}
func (r *purgeFakeRepo) FindRecentByUser(context.Context, string) (ExportJob, error) {
	return ExportJob{}, ErrNotFound
}
func (r *purgeFakeRepo) SetGenerating(context.Context, string) error { return nil }
func (r *purgeFakeRepo) SetReady(context.Context, string, string, int64, time.Time) error {
	return nil
}
func (r *purgeFakeRepo) SetFailed(context.Context, string, string) error { return nil }
func (r *purgeFakeRepo) ExpireOldJobs(context.Context) error             { return nil }
func (r *purgeFakeRepo) PurgeByUser(_ context.Context, _ string) ([]string, error) {
	return r.keys, nil
}

// TestExportService_OpenExportFallsBackToStore: with an empty in-memory cache
// (e.g. after a process restart, or because store-backed exports aren't cached),
// OpenExport must serve the zip from the object store, not return io.EOF.
func TestExportService_OpenExportFallsBackToStore(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	key := "exports/u1/j1.zip"
	up, _ := store.InitMultipart(ctx, key)
	_, _ = store.PutPart(ctx, up, 1, bytes.NewReader([]byte("export-zip-bytes")))
	if _, err := store.CompleteMultipart(ctx, up); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := NewExportService(&purgeFakeRepo{}, nil, nil, store) // cache is empty

	rc, err := svc.OpenExport(ctx, key)
	if err != nil {
		t.Fatalf("OpenExport should fall back to store, got %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "export-zip-bytes" {
		t.Fatalf("served %q, want the stored zip bytes", b)
	}
}

// TestExportService_PurgeUser: PurgeUser deletes the backing object-store zips
// and evicts the in-memory cache for every key the repo purge returned.
func TestExportService_PurgeUser(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	key := "exports/u1/job1.zip"
	up, _ := store.InitMultipart(ctx, key)
	_, _ = store.PutPart(ctx, up, 1, bytes.NewReader([]byte("full pii snapshot")))
	if _, err := store.CompleteMultipart(ctx, up); err != nil {
		t.Fatalf("seed object: %v", err)
	}

	svc := NewExportService(&purgeFakeRepo{keys: []string{key}}, nil, nil, store)
	svc.cache[key] = []byte("full pii snapshot") // simulate the warm cache

	if err := svc.PurgeUser(ctx, "u1"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, ok := svc.cache[key]; ok {
		t.Error("cache entry must be evicted after purge")
	}
	if _, _, err := store.Open(ctx, key); err == nil {
		t.Error("export object must be deleted from the store after purge")
	}
}
