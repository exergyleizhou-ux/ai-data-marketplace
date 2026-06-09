package notification

import (
	"context"
	"fmt"
	"log/slog"
)

// UserLookup resolves a user ID to their email address.
type UserLookup interface {
	EmailOf(ctx context.Context, userID string) (string, error)
}

// Service handles notification creation and retrieval. It also implements
// the Notifier interface so other modules can emit events without importing
// this package — the server wires the service as the Notifier.
type Service struct {
	repo  Repository
	prefs PreferencesRepository
	email EmailSender
	elog  EmailLogRepository
	users UserLookup
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// NewServiceWithChannels creates a notification service with email dispatch.
func NewServiceWithChannels(repo Repository, prefs PreferencesRepository, email EmailSender, elog EmailLogRepository, users UserLookup) *Service {
	return &Service{repo: repo, prefs: prefs, email: email, elog: elog, users: users}
}

// SetEmailChannels wires email dispatch after construction.
func (s *Service) SetEmailChannels(prefs PreferencesRepository, email EmailSender, elog EmailLogRepository, users UserLookup) {
	s.prefs = prefs
	s.email = email
	s.elog = elog
	s.users = users
}

// NotifyUser is the Notifier interface implementation — cross-module emission.
// In-app notification is synchronous; email dispatch is async (go routine).
func (s *Service) NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error {
	// 1. Default prefs: all enabled.
	inApp := true
	doEmail := s.email != nil

	if s.prefs != nil {
		prefs, _ := s.prefs.GetForUser(ctx, userID)
		if p, ok := prefs[kind]; ok {
			inApp = p.InAppEnabled
			doEmail = p.EmailEnabled && s.email != nil
		}
	}

	// 2. In-app: write DB (existing logic).
	if inApp {
		_, err := s.repo.Create(ctx, Notification{
			UserID: userID, Kind: kind, Title: title, Body: body,
			ResourceType: resourceType, ResourceID: resourceID,
		})
		if err != nil {
			slog.Warn("in-app notify failed", "err", err)
		}
	}

	// 3. Email: async (never blocks caller).
	if doEmail {
		idemKey := fmt.Sprintf("%s:%s:%s:%s", userID, resourceType, resourceID, kind)
		go s.sendEmailWithLog(context.Background(), userID, kind, title, body, idemKey)
	}
	return nil
}

func (s *Service) sendEmailWithLog(ctx context.Context, userID, kind, title, body, idemKey string) {
	// Dedup: idempotency key check.
	if s.elog != nil {
		if exists, _ := s.elog.HasKey(ctx, idemKey); exists {
			return
		}
	}

	addr := ""
	if s.users != nil {
		a, err := s.users.EmailOf(ctx, userID)
		if err == nil && a != "" {
			addr = a
		}
	}
	if addr == "" {
		if s.elog != nil {
			_ = s.elog.Log(ctx, userID, kind, "", title, "skipped", "no email address", idemKey)
		}
		return
	}

	subject := "[绿洲] " + title
	htmlBody := "<p>" + body + "</p>"
	textBody := title + "\n\n" + body

	if err := s.email.Send(ctx, addr, subject, htmlBody, textBody); err != nil {
		slog.Warn("email send failed", "user", userID, "kind", kind, "err", err)
		if s.elog != nil {
			_ = s.elog.Log(ctx, userID, kind, addr, title, "failed", err.Error(), idemKey)
		}
		return
	}
	if s.elog != nil {
		_ = s.elog.Log(ctx, userID, kind, addr, title, "sent", "", idemKey)
	}
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

// GetPreferences returns a user's notification preferences.
func (s *Service) GetPreferences(ctx context.Context, userID string) (map[string]NotificationPreference, error) {
	if s.prefs == nil {
		return map[string]NotificationPreference{}, nil
	}
	return s.prefs.GetForUser(ctx, userID)
}

// UpdatePreference updates one preference for a user.
func (s *Service) UpdatePreference(ctx context.Context, userID, kind string, emailEnabled, inAppEnabled bool) error {
	if s.prefs == nil {
		return fmt.Errorf("preferences not available")
	}
	return s.prefs.UpdateForUser(ctx, userID, kind, emailEnabled, inAppEnabled)
}
