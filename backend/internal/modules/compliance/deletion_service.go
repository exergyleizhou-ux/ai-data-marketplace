package compliance

import (
	"context"
	"log/slog"
	"time"
)

// ExportPurger erases a user's data-export jobs + their backing PII zips, so
// account deletion doesn't leave an export archive behind. Implemented by
// *ExportService; optional (nil = no export purge, e.g. in tests).
type ExportPurger interface {
	PurgeUser(ctx context.Context, userID string) error
}

type DeletionService struct {
	repo     DeletionRepository
	notifier Notifier
	exports  ExportPurger
}

func NewDeletionService(repo DeletionRepository, notifier Notifier, exports ExportPurger) *DeletionService {
	return &DeletionService{repo: repo, notifier: notifier, exports: exports}
}

func (s *DeletionService) RequestDeletion(ctx context.Context, userID, reason string) (DeletionRequest, error) {
	coolingUntil := time.Now().Add(7 * 24 * time.Hour)
	req, err := s.repo.Create(ctx, userID, reason, coolingUntil)
	if err != nil {
		return DeletionRequest{}, err
	}
	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, userID, "account_deletion_cooling",
			"账号注销冷静期已开始", "您的账号注销申请已提交，7 天冷静期内可随时撤销。",
			"deletion", req.ID)
	}
	return req, nil
}

func (s *DeletionService) CancelDeletion(ctx context.Context, userID string) (DeletionRequest, error) {
	d, err := s.repo.FindActiveByUser(ctx, userID)
	if err != nil {
		return DeletionRequest{}, err
	}
	if d.Status != DeletionCooling {
		return DeletionRequest{}, ErrDeletionNotCancelable
	}
	return s.repo.Transition(ctx, d.ID, DeletionCooling, DeletionCancelled, userID, "cancelled by user")
}

func (s *DeletionService) Approve(ctx context.Context, opsID, id, note string) (DeletionRequest, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return DeletionRequest{}, err
	}
	if d.Status != DeletionCooling {
		return DeletionRequest{}, ErrBadTransition
	}
	coolingAt, err := time.Parse(time.RFC3339, d.CoolingUntil)
	if err != nil || time.Now().Before(coolingAt) {
		return DeletionRequest{}, ErrCoolingNotElapsed
	}
	r, err := s.repo.Transition(ctx, id, DeletionCooling, DeletionApproved, opsID, note)
	if err != nil {
		return DeletionRequest{}, err
	}
	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, r.UserID, "account_deletion_approved",
			"账号注销已批准", "您的账号注销申请已被批准。",
			"deletion", r.ID)
	}
	return r, nil
}

func (s *DeletionService) Reject(ctx context.Context, opsID, id, reason string) (DeletionRequest, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return DeletionRequest{}, err
	}
	if d.Status != DeletionCooling {
		return DeletionRequest{}, ErrBadTransition
	}
	r, err := s.repo.Transition(ctx, id, DeletionCooling, DeletionRejected, opsID, reason)
	if err != nil {
		return DeletionRequest{}, err
	}
	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, r.UserID, "account_deletion_rejected",
			"账号注销被拒", "您的账号注销申请被拒："+reason,
			"deletion", r.ID)
	}
	return r, nil
}

func (s *DeletionService) Execute(ctx context.Context, opsID, id string) error {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if d.Status != DeletionApproved {
		return ErrBadTransition
	}
	// Erase data-export archives FIRST: each export zip is a full PII snapshot,
	// and the job rows otherwise survive the scrub indefinitely. Done before the
	// scrub so a purge failure aborts (and is retried) rather than leaving an
	// orphaned archive after the user is already scrubbed.
	if s.exports != nil {
		if err := s.exports.PurgeUser(ctx, d.UserID); err != nil {
			return err
		}
	}
	if err := s.repo.ExecuteDeletion(ctx, id, d.UserID, opsID); err != nil {
		return err
	}
	return s.repo.SetDeleted(ctx, id, opsID)
}

func (s *DeletionService) List(ctx context.Context, status string, limit, offset int) ([]DeletionRequest, error) {
	return s.repo.List(ctx, status, limit, offset)
}

// StartScanner periodically expires old export jobs and checks cooling periods.
func (s *ExportService) StartScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.repo.ExpireOldJobs(ctx); err != nil {
					slog.Warn("export expire failed", "err", err)
				}
			}
		}
	}()
}
