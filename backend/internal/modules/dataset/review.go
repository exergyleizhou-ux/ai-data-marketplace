package dataset

import (
	"context"
	"errors"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

var (
	ErrNotReviewable = errors.New("dataset is not awaiting review")
	ErrNotPublished  = errors.New("dataset is not published")
	ErrBadTransition = errors.New("illegal status transition")
)

// allowedTransitions is the dataset lifecycle state machine (docs §5.4). All
// status changes driven by review/delisting go through canTransition so the
// rules live in one place.
var allowedTransitions = map[string]map[string]bool{
	StatusReviewing: {StatusPublished: true, StatusRejected: true},
	StatusPublished: {StatusDelisted: true},
}

func canTransition(from, to string) bool {
	return allowedTransitions[from][to]
}

// Review is the ops decision on a dataset awaiting review: approve -> published,
// reject -> rejected. The note is recorded in the audit trail.
func (s *Service) Review(ctx context.Context, opsID, id string, approve bool, note string) (Dataset, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	if d.Status != StatusReviewing {
		return Dataset{}, ErrNotReviewable
	}
	to := StatusPublished
	action := "dataset.approve"
	if !approve {
		to = StatusRejected
		action = "dataset.reject"
	}
	if !canTransition(d.Status, to) {
		return Dataset{}, ErrBadTransition
	}
	if err := s.repo.SetStatus(ctx, id, to); err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: opsID, ActorRole: "ops", Action: action,
		ResourceType: "dataset", ResourceID: id, Detail: map[string]any{"note": note},
	})
	d.Status = to
	return d, nil
}

// Delist takes a published dataset off the market (ops action / takedown).
func (s *Service) Delist(ctx context.Context, opsID, id, reason string) (Dataset, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	if d.Status != StatusPublished {
		return Dataset{}, ErrNotPublished
	}
	if err := s.repo.SetStatus(ctx, id, StatusDelisted); err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: opsID, ActorRole: "ops", Action: "dataset.delist",
		ResourceType: "dataset", ResourceID: id, Detail: map[string]any{"reason": reason},
	})
	d.Status = StatusDelisted
	return d, nil
}
