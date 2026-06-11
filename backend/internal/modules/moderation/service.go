package moderation

import (
	"context"
	"strings"
)

// Service holds the moderation business rules: validate reports on the way in,
// and gate resolutions to the legal set.
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

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
	return s.repo.Resolve(ctx, id, resolution, opsID)
}
