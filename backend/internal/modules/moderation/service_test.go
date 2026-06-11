package moderation

import (
	"context"
	"testing"
)

type fakeRepo struct {
	created  int
	resolved int
}

func (f *fakeRepo) CreateReport(_ context.Context, _, _, _, _ string) (Report, error) {
	f.created++
	return Report{ID: "r1", Status: StatusOpen}, nil
}
func (f *fakeRepo) ListReports(_ context.Context, status string, _, _ int) ([]Report, error) {
	return []Report{{ID: "r1", Status: status}}, nil
}
func (f *fakeRepo) GetReport(_ context.Context, _ string) (Report, error) { return Report{}, nil }
func (f *fakeRepo) Resolve(_ context.Context, _, _, _ string) (Report, error) {
	f.resolved++
	return Report{ID: "r1", Status: StatusResolved}, nil
}

func TestService_ReportValidation(t *testing.T) {
	f := &fakeRepo{}
	s := NewService(f)
	ctx := context.Background()

	if _, err := s.Report(ctx, "u1", "bogus", "t1", "x"); err != ErrInvalidTarget {
		t.Fatalf("bad target: want ErrInvalidTarget, got %v", err)
	}
	if _, err := s.Report(ctx, "u1", TargetQuestion, "t1", "   "); err != ErrEmptyReason {
		t.Fatalf("blank reason: want ErrEmptyReason, got %v", err)
	}
	if f.created != 0 {
		t.Fatalf("invalid reports must not reach the repo, created=%d", f.created)
	}
	if _, err := s.Report(ctx, "u1", TargetReview, "t1", "real abuse"); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	if f.created != 1 {
		t.Fatalf("valid report should create once, got %d", f.created)
	}
}

func TestService_ResolveValidation(t *testing.T) {
	f := &fakeRepo{}
	s := NewService(f)
	ctx := context.Background()

	if _, err := s.Resolve(ctx, "r1", "delete", "ops1"); err != ErrInvalidResolution {
		t.Fatalf("bad resolution: want ErrInvalidResolution, got %v", err)
	}
	if f.resolved != 0 {
		t.Fatalf("invalid resolution must not reach the repo")
	}
	for _, res := range []string{ResolutionHide, ResolutionDismiss} {
		if _, err := s.Resolve(ctx, "r1", res, "ops1"); err != nil {
			t.Fatalf("valid resolution %q: %v", res, err)
		}
	}
}
