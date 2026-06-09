package watchlist

import (
	"context"
	"sync"
	"testing"
)

// --- fake implementations for service tests ---

type fakeRepo struct {
	// key: "userID|datasetID" → last_notified_version_id
	watches map[string]string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{watches: map[string]string{}} }

func (r *fakeRepo) Add(_ context.Context, userID, datasetID string) error {
	r.watches[userID+"|"+datasetID] = ""
	return nil
}
func (r *fakeRepo) Remove(_ context.Context, userID, datasetID string) error     { return nil }
func (r *fakeRepo) ListByUser(_ context.Context, userID string) ([]Watch, error) { return nil, nil }
func (r *fakeRepo) ListByDataset(_ context.Context, datasetID string) ([]userVersion, error) {
	var out []userVersion
	for k, ver := range r.watches {
		if len(k) > len(datasetID)+1 && k[len(k)-len(datasetID)-1:] == "|"+datasetID {
			userID := k[:len(k)-len(datasetID)-1]
			out = append(out, userVersion{UserID: userID, VersionID: ver})
		}
	}
	return out, nil
}
func (r *fakeRepo) MarkNotified(_ context.Context, userID, datasetID, versionID string) error {
	key := userID + "|" + datasetID
	if _, ok := r.watches[key]; ok {
		r.watches[key] = versionID
	}
	return nil
}

type fakeDSReader struct{ statuses map[string]string }

func (r *fakeDSReader) StatusOf(_ context.Context, dsID string) (string, error) {
	if s, ok := r.statuses[dsID]; ok {
		return s, nil
	}
	return "", ErrNotFound
}

type fakeNotifier struct {
	mu    sync.Mutex
	calls []notifyCall
}
type notifyCall struct {
	UserID, Kind, Title, Body, ResourceType, ResourceID string
}

func (f *fakeNotifier) NotifyUser(_ context.Context, userID, kind, title, body, resourceType, resourceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifyCall{userID, kind, title, body, resourceType, resourceID})
	return nil
}

var _ Notifier = (*fakeNotifier)(nil)

func TestNotifyDatasetPublished_NotifiesAllWatchersAndUpdatesLastNotified(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	svc := NewService(repo, notifier, &fakeDSReader{statuses: map[string]string{"ds1": "published"}})

	// Simulate 3 watchers — u3 already at current version.
	repo.watches["u1|ds1"] = "old-ver"
	repo.watches["u2|ds1"] = "old-ver"
	repo.watches["u3|ds1"] = "new-ver"

	svc.NotifyDatasetPublished(ctx, "ds1", "new-ver", "DS Title")

	if len(notifier.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (u3 already at new-ver)", len(notifier.calls))
	}
	for _, c := range notifier.calls {
		if c.Kind != "dataset_updated" {
			t.Errorf("kind = %q, want dataset_updated", c.Kind)
		}
		if c.ResourceType != "dataset" || c.ResourceID != "ds1" {
			t.Errorf("resource = %s/%s, want dataset/ds1", c.ResourceType, c.ResourceID)
		}
	}
}

func TestNotifyDatasetPublished_NotifierErrorDoesNotBlockOthers(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	repo.watches["u1|ds1"] = "old"
	repo.watches["u2|ds1"] = "old"
	repo.watches["u3|ds1"] = "old"

	notifier := &fakeNotifier{}
	svc := NewService(repo, notifier, &fakeDSReader{statuses: map[string]string{"ds1": "published"}})
	svc.NotifyDatasetPublished(ctx, "ds1", "v1", "T")

	if len(notifier.calls) != 3 {
		t.Fatalf("all 3 watchers must be notified, got %d", len(notifier.calls))
	}
}

func TestAdd_RejectsDraftDataset(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo, nil, &fakeDSReader{statuses: map[string]string{
		"ds-draft":     "draft",
		"ds-uploading": "uploading",
	}})

	if err := svc.Add(ctx, "u1", "ds-draft"); err != ErrNotFound {
		t.Fatalf("draft dataset must return ErrNotFound, got %v", err)
	}
	if err := svc.Add(ctx, "u1", "ds-uploading"); err != ErrNotFound {
		t.Fatalf("uploading dataset must return ErrNotFound, got %v", err)
	}
	// Non-existent dataset ID must also return ErrNotFound.
	if err := svc.Add(ctx, "u1", "ds-nonexistent"); err != ErrNotFound {
		t.Fatalf("non-existent dataset must return ErrNotFound, got %v", err)
	}
	// Published dataset must succeed.
	svc2 := NewService(repo, nil, &fakeDSReader{statuses: map[string]string{"ds-pub": "published"}})
	if err := svc2.Add(ctx, "u1", "ds-pub"); err != nil {
		t.Fatalf("published dataset must succeed, got %v", err)
	}
}
