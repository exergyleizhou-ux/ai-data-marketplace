package apikey

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInvalidKey is returned when a key is unknown or revoked.
	ErrInvalidKey = errors.New("invalid or revoked api key")
	// ErrQuotaExceeded is returned when the key's monthly scan quota is spent.
	ErrQuotaExceeded = errors.New("api key monthly quota exceeded")
	// ErrNotFound is returned when a key id is not owned by the account.
	ErrNotFound = errors.New("api key not found")
)

// Repository persists API keys.
type Repository interface {
	Create(ctx context.Context, k APIKey, keyHash string) (APIKey, error)
	// AuthenticateAndMeter atomically resolves an active key by hash, enforces its
	// tier's monthly quota, increments usage (resetting on a new month), and stamps
	// last_used_at — all under a row lock so concurrent calls can't overspend.
	AuthenticateAndMeter(ctx context.Context, keyHash, month string) (APIKey, error)
	ListByAccount(ctx context.Context, accountID string) ([]APIKey, error)
	Revoke(ctx context.Context, accountID, id string) error
	// SetAccountTier sets the plan tier on ALL of an account's active keys — the
	// target a billing webhook (Stripe subscription change) calls. Returns the
	// number of keys updated.
	SetAccountTier(ctx context.Context, accountID, tier string) (int, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

// NewRepository returns a Postgres-backed Repository.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const keyCols = `id, account_id, name, prefix, tier, usage_month, usage_count,
	created_at::text, COALESCE(last_used_at::text,''), COALESCE(revoked_at::text,'')`

func scanKey(row pgx.Row) (APIKey, error) {
	var k APIKey
	err := row.Scan(&k.ID, &k.AccountID, &k.Name, &k.Prefix, &k.Tier,
		&k.UsageMonth, &k.UsageCount, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
	return k, err
}

func (r *pgRepo) Create(ctx context.Context, k APIKey, keyHash string) (APIKey, error) {
	tier := k.Tier
	if _, ok := Tiers[tier]; !ok {
		tier = "free"
	}
	return scanKey(r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (account_id, name, prefix, key_hash, tier)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+keyCols,
		k.AccountID, k.Name, k.Prefix, keyHash, tier))
}

func (r *pgRepo) AuthenticateAndMeter(ctx context.Context, keyHash, month string) (APIKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return APIKey{}, err
	}
	defer tx.Rollback(ctx)

	k, err := scanKey(tx.QueryRow(ctx,
		`SELECT `+keyCols+` FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL FOR UPDATE`, keyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIKey{}, ErrInvalidKey
		}
		return APIKey{}, err
	}

	// New calendar month → counter resets to 1; otherwise enforce the tier quota.
	newCount := 1
	if k.UsageMonth == month {
		if k.UsageCount >= TierOf(k.Tier).MonthlyQuota {
			return APIKey{}, ErrQuotaExceeded
		}
		newCount = k.UsageCount + 1
	}

	if _, err := tx.Exec(ctx,
		`UPDATE api_keys SET usage_count=$2, usage_month=$3, last_used_at=now() WHERE id=$1`,
		k.ID, newCount, month); err != nil {
		return APIKey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return APIKey{}, err
	}
	k.UsageCount, k.UsageMonth = newCount, month
	return k, nil
}

func (r *pgRepo) ListByAccount(ctx context.Context, accountID string) ([]APIKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+keyCols+` FROM api_keys WHERE account_id=$1 ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *pgRepo) Revoke(ctx context.Context, accountID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND account_id=$2 AND revoked_at IS NULL`,
		id, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgRepo) SetAccountTier(ctx context.Context, accountID, tier string) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET tier=$2 WHERE account_id=$1 AND revoked_at IS NULL`, accountID, tier)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
