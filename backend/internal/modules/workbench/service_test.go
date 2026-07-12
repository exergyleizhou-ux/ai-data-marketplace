package workbench

import (
	"context"
	"testing"
)

type fakeRepo struct{ owned bool }

func (fakeRepo) GetOrCreatePersonalWorkspace(context.Context, string) (Workspace, error) {
	return Workspace{ID: "personal"}, nil
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
