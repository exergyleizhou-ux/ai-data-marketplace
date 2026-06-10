package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecordLoginFailure atomically increments the account's consecutive-failure
// counter (restarting it if a previous lock has expired) and sets locked_until
// once the counter reaches maxFailures. Returns the new counter and the lock
// expiry (zero time when not locked). Single UPSERT — race-safe under
// concurrent failed logins.
func (r *pgRepo) RecordLoginFailure(ctx context.Context, userID string, maxFailures int, lockFor time.Duration) (int, time.Time, error) {
	const q = `
		INSERT INTO login_lockouts (user_id, failures, locked_until)
		VALUES ($1::uuid, 1, CASE WHEN 1 >= $2::int THEN now() + $3::interval END)
		ON CONFLICT (user_id) DO UPDATE SET
			failures = CASE
				WHEN login_lockouts.locked_until IS NOT NULL AND login_lockouts.locked_until <= now() THEN 1
				ELSE login_lockouts.failures + 1 END,
			locked_until = CASE
				WHEN (CASE
					WHEN login_lockouts.locked_until IS NOT NULL AND login_lockouts.locked_until <= now() THEN 1
					ELSE login_lockouts.failures + 1 END) >= $2::int
				THEN now() + $3::interval END,
			updated_at = now()
		RETURNING failures, COALESCE(locked_until, 'epoch'::timestamptz)`
	var n int
	var until time.Time
	err := r.pool.QueryRow(ctx, q, userID, maxFailures, lockFor.String()).Scan(&n, &until)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("record login failure: %w", err)
	}
	if until.Unix() <= 0 {
		until = time.Time{}
	}
	return n, until, nil
}

// LoginLockedUntil reports whether the account is currently locked out.
func (r *pgRepo) LoginLockedUntil(ctx context.Context, userID string) (time.Time, bool, error) {
	const q = `SELECT locked_until FROM login_lockouts
		WHERE user_id=$1::uuid AND locked_until IS NOT NULL AND locked_until > now()`
	var until time.Time
	err := r.pool.QueryRow(ctx, q, userID).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("login locked until: %w", err)
	}
	return until, true, nil
}

// ClearLoginFailures resets the account's counter (successful login or
// completed password reset).
func (r *pgRepo) ClearLoginFailures(ctx context.Context, userID string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM login_lockouts WHERE user_id=$1::uuid`, userID); err != nil {
		return fmt.Errorf("clear login failures: %w", err)
	}
	return nil
}
