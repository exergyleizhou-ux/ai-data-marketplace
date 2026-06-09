package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationPreference is one user's preference for a notification kind.
type NotificationPreference struct {
	Kind         string `json:"kind"`
	EmailEnabled bool   `json:"email_enabled"`
	InAppEnabled bool   `json:"in_app_enabled"`
}

// PreferencesRepository stores per-user notification preferences.
type PreferencesRepository interface {
	GetForUser(ctx context.Context, userID string) (map[string]NotificationPreference, error)
	UpdateForUser(ctx context.Context, userID, kind string, emailEnabled, inAppEnabled bool) error
}

// EmailLogRepository records sent/skipped/failed emails for dedup and debugging.
type EmailLogRepository interface {
	HasKey(ctx context.Context, idempotencyKey string) (bool, error)
	Log(ctx context.Context, userID, kind, to, subject, status, errMsg, idempotencyKey string) error
}

type pgPrefsRepo struct{ pool *pgxpool.Pool }

func NewPreferencesRepository(pool *pgxpool.Pool) PreferencesRepository {
	return &pgPrefsRepo{pool: pool}
}

func (r *pgPrefsRepo) GetForUser(ctx context.Context, userID string) (map[string]NotificationPreference, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT kind, email_enabled, in_app_enabled
		 FROM notification_preferences WHERE user_id=$1`, userID)
	if err != nil {
		return nil, fmt.Errorf("get prefs: %w", err)
	}
	defer rows.Close()
	out := map[string]NotificationPreference{}
	for rows.Next() {
		var p NotificationPreference
		if err := rows.Scan(&p.Kind, &p.EmailEnabled, &p.InAppEnabled); err != nil {
			return nil, err
		}
		out[p.Kind] = p
	}
	return out, rows.Err()
}

func (r *pgPrefsRepo) UpdateForUser(ctx context.Context, userID, kind string, emailEnabled, inAppEnabled bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notification_preferences (user_id, kind, email_enabled, in_app_enabled, updated_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (user_id, kind) DO UPDATE
		 SET email_enabled=EXCLUDED.email_enabled, in_app_enabled=EXCLUDED.in_app_enabled, updated_at=EXCLUDED.updated_at`,
		userID, kind, emailEnabled, inAppEnabled, time.Now())
	if err != nil {
		return fmt.Errorf("update pref: %w", err)
	}
	return nil
}

type pgEmailLogRepo struct{ pool *pgxpool.Pool }

func NewEmailLogRepository(pool *pgxpool.Pool) EmailLogRepository { return &pgEmailLogRepo{pool: pool} }

func (r *pgEmailLogRepo) HasKey(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_send_log WHERE idempotency_key=$1)`, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check email key: %w", err)
	}
	return exists, nil
}

func (r *pgEmailLogRepo) Log(ctx context.Context, userID, kind, to, subject, status, errMsg, idempotencyKey string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO email_send_log (user_id, kind, to_address, subject, status, error, idempotency_key)
		 VALUES ($1::uuid,$2,$3,$4,$5,NULLIF($6,''),$7)
		 ON CONFLICT (idempotency_key) DO NOTHING`,
		userID, kind, to, subject, status, errMsg, idempotencyKey)
	if err != nil {
		return fmt.Errorf("log email: %w", err)
	}
	return nil
}
