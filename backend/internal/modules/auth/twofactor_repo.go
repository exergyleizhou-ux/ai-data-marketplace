package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// --- 2FA TOTP ---

func (r *pgRepo) SetTOTPSecret(ctx context.Context, userID, secret string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET totp_secret=$2 WHERE id=$1`, userID, secret)
	if err != nil {
		return fmt.Errorf("set totp secret: %w", err)
	}
	return nil
}

func (r *pgRepo) EnableTOTP(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET totp_enabled=true WHERE id=$1`, userID)
	if err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}
	return nil
}

func (r *pgRepo) GetTOTPSecret(ctx context.Context, userID string) (string, error) {
	var secret sql.NullString
	err := r.pool.QueryRow(ctx, `SELECT totp_secret FROM users WHERE id=$1`, userID).Scan(&secret)
	if err != nil {
		return "", fmt.Errorf("get totp secret: %w", err)
	}
	return secret.String, nil
}

func (r *pgRepo) AddRecoveryCode(ctx context.Context, userID, codeHash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO totp_recovery_codes (user_id, code_hash) VALUES ($1,$2)
		 ON CONFLICT (user_id, code_hash) DO NOTHING`, userID, codeHash)
	if err != nil {
		return fmt.Errorf("add recovery code: %w", err)
	}
	return nil
}

// ClearRecoveryCodes removes all of a user's recovery codes. Used before
// re-issuing on enrollment so an abandoned enrollment's codes don't stay valid.
func (r *pgRepo) ClearRecoveryCodes(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM totp_recovery_codes WHERE user_id=$1`, userID)
	if err != nil {
		return fmt.Errorf("clear recovery codes: %w", err)
	}
	return nil
}

func (r *pgRepo) ConsumeRecoveryCode(ctx context.Context, userID, plaintext string) (bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT code_hash FROM totp_recovery_codes WHERE user_id=$1 AND used_at IS NULL`, userID)
	if err != nil {
		return false, fmt.Errorf("fetch recovery codes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var codeHash string
		if err := rows.Scan(&codeHash); err != nil {
			return false, err
		}
		if bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(plaintext)) == nil {
			// Atomically mark used.  UPDATE ... AND used_at IS NULL prevents TOCTOU.
			tag, _ := r.pool.Exec(ctx,
				`UPDATE totp_recovery_codes SET used_at=now()
				 WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`,
				userID, codeHash)
			if tag.RowsAffected() == 0 {
				// Another concurrent request consumed this code first.
				return false, nil
			}
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *pgRepo) DisableTOTP(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET totp_enabled=false, totp_secret=NULL WHERE id=$1;
		 DELETE FROM totp_recovery_codes WHERE user_id=$1`, userID)
	if err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	return nil
}

func (r *pgRepo) CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM totp_recovery_codes WHERE user_id=$1 AND used_at IS NULL`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count recovery codes: %w", err)
	}
	return n, nil
}

// --- Password reset ---

func (r *pgRepo) CreatePasswordResetToken(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
		 VALUES ($1,$2,$3)`, tokenHash, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("create reset token: %w", err)
	}
	return nil
}

func (r *pgRepo) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx,
		`UPDATE password_reset_tokens SET used_at=now()
		 WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now()
		 RETURNING user_id::text`, tokenHash).Scan(&userID)
	if err != nil {
		return "", ErrTokenInvalidOrExpired
	}
	return userID, nil
}

func (r *pgRepo) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash=$2 WHERE id=$1`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// InvalidateSessions stamps the user's session-invalidation epoch so every
// refresh token issued before now is rejected on the next refresh. (The prior
// implementation updated a non-existent refresh_tokens table and its error was
// swallowed, so a password reset terminated no sessions at all.)
func (r *pgRepo) InvalidateSessions(ctx context.Context, userID string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE users SET tokens_valid_after = now() WHERE id=$1`, userID); err != nil {
		return fmt.Errorf("invalidate sessions: %w", err)
	}
	return nil
}
