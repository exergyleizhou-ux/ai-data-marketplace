package moderation

import (
	"context"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// Service holds the moderation business rules: validate reports on the way in,
// and gate resolutions to the legal set.
type Service struct {
	repo  Repository
	audit audit.Recorder
}

func NewService(repo Repository, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	return &Service{repo: repo, audit: rec}
}

// Report files a user report against a question or review.
func (s *Service) Report(ctx context.Context, reporterID, targetType, targetID, reason string) (Report, error) {
	if !validTarget(targetType) {
		return Report{}, ErrInvalidTarget
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Report{}, ErrEmptyReason
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return s.repo.CreateReport(ctx, reporterID, targetType, targetID, reason)
}

// ListReports returns reports for the ops console (status="" => all).
func (s *Service) ListReports(ctx context.Context, status string, limit, offset int) ([]Report, error) {
	if status != "" && status != StatusOpen && status != StatusResolved {
		status = "" // ignore unknown filter rather than 400 the ops console
	}
	return s.repo.ListReports(ctx, status, limit, offset)
}

// Resolve closes an open report, optionally hiding the reported content.
func (s *Service) Resolve(ctx context.Context, id, resolution, opsID string) (Report, error) {
	if !validResolution(resolution) {
		return Report{}, ErrInvalidResolution
	}
	out, err := s.repo.Resolve(ctx, id, resolution, opsID)
	if err != nil {
		return Report{}, err
	}
	// A content takedown/dismissal is a privileged, dispute-ruling-class action
	// (audit §6.8) — record it in the append-only trail so it isn't invisible
	// (the only other trace is the mutable resolved_by column).
	s.audit.Record(ctx, audit.Entry{
		ActorID: opsID, Action: "moderation.resolve",
		ResourceType: out.TargetType, ResourceID: out.TargetID,
		Detail: map[string]any{"resolution": resolution, "report_id": id},
	})
	return out, nil
}
