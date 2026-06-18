package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresDenylist stores revoked refresh-token jtis in Postgres so revocation
// is durable (survives restarts) and shared across all instances — unlike the
// in-memory fallback, where a logout/rotation on one instance leaves the token
// valid on another and is lost on restart. Entries are kept only until the
// token's own expiry; IsRevoked ignores expired rows and a cleaner purges them.
type PostgresDenylist struct{ pool *pgxpool.Pool }

// NewPostgresDenylist wraps a pgx pool.
func NewPostgresDenylist(pool *pgxpool.Pool) *PostgresDenylist {
	return &PostgresDenylist{pool: pool}
}

// Revoke records jti as revoked until now+ttl. A ttl <= 0 is a no-op (the
// token has already expired and cannot be used regardless). Re-revoking an
// existing jti is idempotent.
func (d *PostgresDenylist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" || ttl <= 0 {
		return nil
	}
	_, err := d.pool.Exec(ctx,
		`INSERT INTO revoked_refresh_tokens (jti, expires_at) VALUES ($1, $2)
		 ON CONFLICT (jti) DO UPDATE SET expires_at = EXCLUDED.expires_at`,
		jti, time.Now().UTC().Add(ttl))
	return err
}

// IsRevoked reports whether jti is currently revoked. Expired rows are treated
// as not revoked (the token is dead on its own anyway).
func (d *PostgresDenylist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	var revoked bool
	err := d.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM revoked_refresh_tokens WHERE jti = $1 AND expires_at > now())`,
		jti).Scan(&revoked)
	return revoked, err
}
