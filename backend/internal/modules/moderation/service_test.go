package moderation

import (
	"context"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
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
	return Report{ID: "r1", Status: StatusResolved, TargetType: TargetQuestion, TargetID: "q-7"}, nil
}

type fakeRecorder struct{ entries []audit.Entry }

func (r *fakeRecorder) Record(_ context.Context, e audit.Entry) { r.entries = append(r.entries, e) }

func TestService_ReportValidation(t *testing.T) {
	f := &fakeRepo{}
	s := NewService(f, nil)
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
	s := NewService(f, nil)
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

// TestService_ResolveRecordsAudit: a privileged hide/dismiss must leave an
// append-only audit entry (it previously left none — only the mutable
// resolved_by column).
func TestService_ResolveRecordsAudit(t *testing.T) {
	f := &fakeRepo{}
	rec := &fakeRecorder{}
	s := NewService(f, rec)

	if _, err := s.Resolve(context.Background(), "r1", ResolutionHide, "ops1"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Action != "moderation.resolve" || e.ActorID != "ops1" || e.ResourceID != "q-7" {
		t.Fatalf("audit entry wrong: %+v", e)
	}
	// An invalid resolution is rejected before the repo and must NOT be audited.
	if _, err := s.Resolve(context.Background(), "r1", "delete", "ops1"); err != ErrInvalidResolution {
		t.Fatalf("want ErrInvalidResolution, got %v", err)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("rejected resolution must not be audited, entries=%d", len(rec.entries))
	}
}
