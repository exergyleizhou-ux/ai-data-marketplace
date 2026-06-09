package notification

import (
	"context"
	"fmt"
	"log/slog"
)

// Service handles notification creation and retrieval. It also implements
// the Notifier interface so other modules can emit events without importing
// this package — the server wires the service as the Notifier.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// NotifyUser is the Notifier interface implementation — cross-module emission.
func (s *Service) NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error {
	_, err := s.repo.Create(ctx, Notification{
		UserID: userID, Kind: kind, Title: title, Body: body,
		ResourceType: resourceType, ResourceID: resourceID,
	})
	if err != nil {
		// Don't fail the caller's transaction because of a notification hiccup.
		slog.Error("notification create failed", "user_id", userID, "kind", kind, "err", err)
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// List returns a user's notifications, newest first.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Notification, error) {
	return s.repo.ListByUser(ctx, userID, limit, offset)
}

// MarkRead marks one notification as read (must belong to the user).
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkRead(ctx, id, userID)
}

// MarkAllRead marks all of a user's notifications as read.
func (s *Service) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	return s.repo.MarkAllRead(ctx, userID)
}

// CountUnread returns the number of unread notifications for a user.
func (s *Service) CountUnread(ctx context.Context, userID string) (int64, error) {
	return s.repo.CountUnread(ctx, userID)
}
