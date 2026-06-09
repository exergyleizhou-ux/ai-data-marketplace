package notification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// --- fakes for service email tests ---

type fakeNotifPrefsRepo struct {
	prefs map[string]map[string]NotificationPreference // userID → kind → pref
}

func (r *fakeNotifPrefsRepo) GetForUser(_ context.Context, userID string) (map[string]NotificationPreference, error) {
	if r.prefs == nil {
		return map[string]NotificationPreference{}, nil
	}
	m, ok := r.prefs[userID]
	if !ok {
		return map[string]NotificationPreference{}, nil
	}
	return m, nil
}
func (r *fakeNotifPrefsRepo) UpdateForUser(_ context.Context, _, _ string, _, _ bool) error {
	return nil
}

type fakeEmailLogRepo struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (r *fakeEmailLogRepo) HasKey(_ context.Context, key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.keys[key], nil
}
func (r *fakeEmailLogRepo) Log(_ context.Context, _, _, _, _, _, _, _ string) error { return nil }

type fakeUserLookup struct{ emails map[string]string }

func (f *fakeUserLookup) EmailOf(_ context.Context, userID string) (string, error) {
	if e, ok := f.emails[userID]; ok {
		return e, nil
	}
	return "", errors.New("no email")
}

// --- tests ---

func TestNotifyUser_RespectsEmailDisabledPref(t *testing.T) {
	ctx := context.Background()
	mock := &MockSender{}
	svc := &Service{
		repo: &fakeRepo{},
		prefs: &fakeNotifPrefsRepo{prefs: map[string]map[string]NotificationPreference{
			"u1": {"order_paid": {EmailEnabled: false, InAppEnabled: true}},
		}},
		email: mock,
		elog:  &fakeEmailLogRepo{keys: map[string]bool{}},
		users: &fakeUserLookup{emails: map[string]string{"u1": "u1@x.com"}},
	}
	_ = svc.NotifyUser(ctx, "u1", "order_paid", "title", "body", "order", "o1")
	time.Sleep(30 * time.Millisecond)
	if len(mock.Sent) != 0 {
		t.Fatalf("email sent = %d, want 0 (email disabled)", len(mock.Sent))
	}
}

func TestNotifyUser_RespectsInAppDisabledPref(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{}
	svc := &Service{
		repo: repo,
		prefs: &fakeNotifPrefsRepo{prefs: map[string]map[string]NotificationPreference{
			"u1": {"order_paid": {EmailEnabled: true, InAppEnabled: false}},
		}},
		email: &MockSender{},
		elog:  &fakeEmailLogRepo{keys: map[string]bool{}},
		users: &fakeUserLookup{emails: map[string]string{"u1": "u1@x.com"}},
	}
	_ = svc.NotifyUser(ctx, "u1", "order_paid", "title", "body", "order", "o1")
	time.Sleep(30 * time.Millisecond)
	if repo.createCalled {
		t.Fatal("in-app notification must not be created when disabled")
	}
}

func TestNotifyUser_SkipsEmailWhenUserHasNoAddress(t *testing.T) {
	ctx := context.Background()
	mock := &MockSender{}
	svc := &Service{
		repo:  &fakeRepo{},
		prefs: &fakeNotifPrefsRepo{},
		email: mock,
		elog:  &fakeEmailLogRepo{keys: map[string]bool{}},
		users: &fakeUserLookup{emails: map[string]string{}},
	}
	_ = svc.NotifyUser(ctx, "u1", "order_paid", "title", "body", "order", "o1")
	time.Sleep(30 * time.Millisecond)
	// async — need a tiny wait for the goroutine.
	// For deterministic test, mock no email → sendEmailWithLog logs 'skipped'.
	// We just verify no panic and mock.Sent stays 0.
	if len(mock.Sent) != 0 {
		t.Fatalf("email sent without address: %d", len(mock.Sent))
	}
}

// fakeRepo with a createCalled flag for in-app disable test.
type fakeRepo struct {
	createCalled bool
}

func (r *fakeRepo) Create(_ context.Context, n Notification) (Notification, error) {
	r.createCalled = true
	return n, nil
}
func (r *fakeRepo) ListByUser(ctx context.Context, s string, i1, i2 int) ([]Notification, error) {
	return nil, nil
}
func (r *fakeRepo) MarkRead(ctx context.Context, s1, s2 string) error        { return nil }
func (r *fakeRepo) MarkAllRead(ctx context.Context, s string) (int64, error) { return 0, nil }
func (r *fakeRepo) CountUnread(ctx context.Context, s string) (int64, error) { return 0, nil }

func TestNotifyUser_PreventsDoubleEmailViaIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	mock := &MockSender{}
	keys := map[string]bool{"u1:order:o1:order_paid": true} // already sent
	svc := &Service{
		repo:  &fakeRepo{},
		prefs: &fakeNotifPrefsRepo{},
		email: mock,
		elog:  &fakeEmailLogRepo{keys: keys},
		users: &fakeUserLookup{emails: map[string]string{"u1": "u1@x.com"}},
	}

	_ = svc.NotifyUser(ctx, "u1", "order_paid", "title", "body", "order", "o1")
	time.Sleep(30 * time.Millisecond)
	if len(mock.Sent) != 0 {
		t.Fatalf("idempotency key must prevent duplicate: sent = %d", len(mock.Sent))
	}
}

func TestNotifyUser_SMTPFailureLogsButDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	mock := &MockSender{Fail: true}
	svc := &Service{
		repo:  &fakeRepo{},
		prefs: &fakeNotifPrefsRepo{},
		email: mock,
		elog:  &fakeEmailLogRepo{keys: map[string]bool{}},
		users: &fakeUserLookup{emails: map[string]string{"u1": "u1@x.com"}},
	}
	err := svc.NotifyUser(ctx, "u1", "order_paid", "title", "body", "order", "o1")
	time.Sleep(30 * time.Millisecond)
	if err != nil {
		t.Fatalf("NotifyUser must never return error for SMTP failure, got: %v", err)
	}
}
