package workbench

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	owned  bool
	active *bool
}

func (f fakeRepo) AccountActive(context.Context, string) (bool, error) {
	if f.active == nil {
		return true, nil
	}
	return *f.active, nil
}

func (fakeRepo) GetOrCreatePersonalWorkspace(context.Context, string) (Workspace, error) {
	return Workspace{ID: "personal"}, nil
}
func TestServiceRejectsFrozenAccountBeforeIssuingWorkbenchToken(t *testing.T) {
	inactive := false
	s := NewService(fakeRepo{owned: true, active: &inactive}, NewTokenManager("s", 5*time.Minute))
	if _, err := s.Issue(context.Background(), "frozen-user", "workspace"); !errors.Is(err, ErrAccountInactive) {
		t.Fatalf("Issue frozen account error = %v, want ErrAccountInactive", err)
	}
}
func (f fakeRepo) GetOwned(_ context.Context, id, uid string) (Workspace, error) {
	if !f.owned {
		return Workspace{}, ErrNotFound
	}
	return Workspace{ID: id, AccountID: uid}, nil
}
func TestServiceRejectsUnownedWorkspace(t *testing.T) {
	s := NewService(fakeRepo{}, NewTokenManager("s", 0))
	if _, e := s.Issue(context.Background(), "u", "other"); e != ErrNotFound {
		t.Fatalf("err=%v", e)
	}
}
