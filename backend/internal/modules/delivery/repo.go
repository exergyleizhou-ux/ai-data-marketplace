package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the delivery table.
type Repository interface {
	// Upsert issues (or re-issues) the download grant for an order, resetting
	// the token, expiry and counter.
	Upsert(ctx context.Context, orderID, tokenHash string, expiresAt time.Time, maxDownloads int, fingerprint string) error
	GetByTokenHash(ctx context.Context, tokenHash string) (Grant, error)
	// ConsumeDownload atomically increments the counter iff under the limit,
	// recording the client IP. Returns false if the quota is exhausted.
	ConsumeDownload(ctx context.Context, id, ip string) (bool, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) Upsert(ctx context.Context, orderID, tokenHash string, expiresAt time.Time, maxDownloads int, fingerprint string) error {
	const q = `
		INSERT INTO deliveries (order_id, download_token_hash, expires_at, max_downloads, download_count, delivery_fingerprint)
		VALUES ($1,$2,$3,$4,0,$5)
		ON CONFLICT (order_id) DO UPDATE
		SET download_token_hash=EXCLUDED.download_token_hash,
		    expires_at=EXCLUDED.expires_at,
		    max_downloads=EXCLUDED.max_downloads,
		    download_count=0,
		    delivery_fingerprint=EXCLUDED.delivery_fingerprint`
	if _, err := r.pool.Exec(ctx, q, orderID, tokenHash, expiresAt, maxDownloads, fingerprint); err != nil {
		return fmt.Errorf("upsert delivery: %w", err)
	}
	return nil
}

func (r *pgRepo) GetByTokenHash(ctx context.Context, tokenHash string) (Grant, error) {
	const q = `
		SELECT id, order_id, expires_at, max_downloads, download_count, COALESCE(delivery_fingerprint,'')
		FROM deliveries WHERE download_token_hash=$1`
	var g Grant
	err := r.pool.QueryRow(ctx, q, tokenHash).
		Scan(&g.ID, &g.OrderID, &g.ExpiresAt, &g.MaxDownloads, &g.DownloadCount, &g.Fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return Grant{}, ErrTokenInvalid
	}
	if err != nil {
		return Grant{}, fmt.Errorf("get delivery: %w", err)
	}
	return g, nil
}

func (r *pgRepo) ConsumeDownload(ctx context.Context, id, ip string) (bool, error) {
	const q = `
		UPDATE deliveries
		SET download_count = download_count + 1, last_download_ip = NULLIF($2,'')::inet
		WHERE id=$1 AND download_count < max_downloads AND expires_at > now()`
	ct, err := r.pool.Exec(ctx, q, id, ip)
	if err != nil {
		return false, fmt.Errorf("consume download: %w", err)
	}
	return ct.RowsAffected() == 1, nil
}
