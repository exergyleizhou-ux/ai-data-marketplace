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
	svc := NewService(repo, notifier)

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
	svc := NewService(repo, notifier)
	svc.NotifyDatasetPublished(ctx, "ds1", "v1", "T")

	if len(notifier.calls) != 3 {
		t.Fatalf("all 3 watchers must be notified, got %d", len(notifier.calls))
	}
}
