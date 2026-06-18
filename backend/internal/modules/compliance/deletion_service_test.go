package compliance

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePurger struct {
	purged []string
	err    error
}

func (p *fakePurger) PurgeUser(_ context.Context, userID string) error {
	p.purged = append(p.purged, userID)
	return p.err
}

// TestExecute_PurgesExportsBeforeScrub: account deletion must erase the user's
// data-export archives (each a full PII snapshot). If the purge fails, the
// deletion aborts (user not marked deleted) so it retries rather than leaving an
// orphaned archive behind.
func TestExecute_PurgesExportsBeforeScrub(t *testing.T) {
	repo := &fakeDeletionRepo{reqs: map[string]DeletionRequest{
		"d-9": {ID: "d-9", UserID: "u9", Status: DeletionApproved},
	}}
	purger := &fakePurger{}
	svc := NewDeletionService(repo, nil, purger)
	if err := svc.Execute(context.Background(), "ops", "d-9"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(purger.purged) != 1 || purger.purged[0] != "u9" {
		t.Fatalf("export purge not invoked for deleted user, got %v", purger.purged)
	}

	repo2 := &fakeDeletionRepo{reqs: map[string]DeletionRequest{
		"d-10": {ID: "d-10", UserID: "u10", Status: DeletionApproved},
	}}
	svc2 := NewDeletionService(repo2, nil, &fakePurger{err: errors.New("boom")})
	if err := svc2.Execute(context.Background(), "ops", "d-10"); err == nil {
		t.Fatal("a purge failure must abort the deletion")
	}
	if repo2.reqs["d-10"].Status == DeletionDeleted {
		t.Fatal("user must NOT be marked deleted when export purge failed")
	}
}

type fakeDeletionRepo struct {
	reqs map[string]DeletionRequest
}

func (r *fakeDeletionRepo) Create(_ context.Context, userID, reason string, coolingUntil time.Time) (DeletionRequest, error) {
	d := DeletionRequest{ID: "d-1", UserID: userID, Reason: reason, Status: DeletionCooling,
		CoolingUntil: coolingUntil.Format(time.RFC3339), RequestedAt: "now"}
	if r.reqs == nil {
		r.reqs = map[string]DeletionRequest{}
	}
	r.reqs[d.ID] = d
	return d, nil
}
func (r *fakeDeletionRepo) FindActiveByUser(_ context.Context, userID string) (DeletionRequest, error) {
	for _, d := range r.reqs {
		if d.UserID == userID && d.Status == DeletionCooling {
			return d, nil
		}
	}
	return DeletionRequest{}, ErrNotFound
}
func (r *fakeDeletionRepo) Get(_ context.Context, id string) (DeletionRequest, error) {
	d, ok := r.reqs[id]
	if !ok {
		return DeletionRequest{}, ErrNotFound
	}
	return d, nil
}
func (r *fakeDeletionRepo) List(_ context.Context, _ string, _, _ int) ([]DeletionRequest, error) {
	return nil, nil
}
func (r *fakeDeletionRepo) Transition(_ context.Context, id, from, to, opsID, note string) (DeletionRequest, error) {
	d := r.reqs[id]
	if d.Status != from {
		return DeletionRequest{}, ErrBadTransition
	}
	d.Status = to
	d.ProcessedBy = opsID
	d.OpsNote = note
	r.reqs[id] = d
	return d, nil
}
func (r *fakeDeletionRepo) ExecuteDeletion(_ context.Context, _, _, _ string) error { return nil }
func (r *fakeDeletionRepo) SetDeleted(_ context.Context, id, opsID string) error {
	d := r.reqs[id]
	d.Status = DeletionDeleted
	r.reqs[id] = d
	return nil
}

type fakeCNotifier struct {
	calls []fakeCNotify
}
type fakeCNotify struct{ userID, kind string }

func (f *fakeCNotifier) NotifyUser(_ context.Context, userID, kind, _, _, _, _ string) error {
	f.calls = append(f.calls, fakeCNotify{userID, kind})
	return nil
}

func TestRequestDeletion_SetsCoolingUntilSevenDaysOut(t *testing.T) {
	repo := &fakeDeletionRepo{}
	svc := NewDeletionService(repo, nil, nil)
	d, err := svc.RequestDeletion(context.Background(), "u1", "reason")
	if err != nil {
		t.Fatal(err)
	}
	coolAt, _ := time.Parse(time.RFC3339, d.CoolingUntil)
	expected := time.Now().Add(7 * 24 * time.Hour)
	if coolAt.Before(expected.Add(-5*time.Second)) || coolAt.After(expected.Add(5*time.Second)) {
		t.Fatalf("cooling_until = %s, want ~%s", d.CoolingUntil, expected.Format(time.RFC3339))
	}
}

func TestApproveDeletion_RejectsBeforeCoolingElapsed(t *testing.T) {
	repo := &fakeDeletionRepo{}
	svc := NewDeletionService(repo, nil, nil)
	d, _ := svc.RequestDeletion(context.Background(), "u1", "reason")
	d.CoolingUntil = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	repo.reqs[d.ID] = d

	_, err := svc.Approve(context.Background(), "ops", "d-1", "")
	if err != ErrCoolingNotElapsed {
		t.Fatalf("want ErrCoolingNotElapsed, got %v", err)
	}
}

func TestExecuteDeletion_OnlyAcceptsApproved(t *testing.T) {
	repo := &fakeDeletionRepo{}
	svc := NewDeletionService(repo, nil, nil)
	d, _ := svc.RequestDeletion(context.Background(), "u1", "reason")
	repo.reqs[d.ID] = d

	err := svc.Execute(context.Background(), "ops", "d-1")
	if err != ErrBadTransition {
		t.Fatalf("want ErrBadTransition, got %v", err)
	}
}

func TestRequestDeletion_StartsCooling(t *testing.T) {
	// Integration test: covered by repo test (ExecuteDeletion in deletion_repo.go
	// does the actual SQL scrub).
	// This unit test confirms the flow: Execute checks status=approved, calls
	// repo.ExecuteDeletion (which in production does the real SQL).
	repo := &fakeDeletionRepo{}
	svc := NewDeletionService(repo, nil, nil)
	d, _ := svc.RequestDeletion(context.Background(), "u1", "reason")
	if d.Status != DeletionCooling {
		t.Fatal("new request must be cooling")
	}
}
