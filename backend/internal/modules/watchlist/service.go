package watchlist

import (
	"context"
	"log/slog"
)

// Notifier is the notification interface used by this module.
type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

// Service handles watch add/remove/list and dataset-publish notification.
type Service struct {
	repo     Repository
	notifier Notifier
}

func NewService(repo Repository, notifier Notifier) *Service {
	return &Service{repo: repo, notifier: notifier}
}

// Add creates a watch. The dataset must exist and be reviewable/published.
// Idempotent (ON CONFLICT DO NOTHING).
func (s *Service) Add(ctx context.Context, userID, datasetID string) error {
	return s.repo.Add(ctx, userID, datasetID)
}

// Remove deletes a watch. Idempotent (no-op if not found).
func (s *Service) Remove(ctx context.Context, userID, datasetID string) error {
	return s.repo.Remove(ctx, userID, datasetID)
}

// ListMy returns all watches belonging to a user.
func (s *Service) ListMy(ctx context.Context, userID string) ([]Watch, error) {
	return s.repo.ListByUser(ctx, userID)
}

// NotifyDatasetPublished is called by the dataset module after a dataset is
// published. It notifies all watchers whose last_notified_version_id differs
// from newVersionID, then marks them notified. Must be called asynchronously
// (go NotifyDatasetPublished(...)) so it never blocks ops review.
func (s *Service) NotifyDatasetPublished(ctx context.Context, datasetID, newVersionID, datasetTitle string) {
	uvs, err := s.repo.ListByDataset(ctx, datasetID)
	if err != nil {
		slog.Warn("watchlist: ListByDataset failed", "dataset", datasetID, "err", err)
		return
	}
	for _, uv := range uvs {
		if uv.VersionID == newVersionID {
			continue // already notified for this version
		}
		// NotifyUser failure does not block other watchers.
		if s.notifier != nil {
			_ = s.notifier.NotifyUser(ctx, uv.UserID, "dataset_updated",
				"关注的数据集有更新", "数据集「"+datasetTitle+"」已发布新版本。",
				"dataset", datasetID)
		}
		// MarkNotified failure logged but does not block other watchers.
		if err := s.repo.MarkNotified(ctx, uv.UserID, datasetID, newVersionID); err != nil {
			slog.Warn("watchlist: MarkNotified failed", "user", uv.UserID, "dataset", datasetID, "err", err)
		}
	}
}
