package verify

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository reads the certificates lookup table.
type Repository interface {
	FindByCertID(ctx context.Context, certID string) (*CertInfo, error)
	// Register persists a certificate idempotently.
	Register(ctx context.Context, certID, resourceType, resourceID string) error
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) FindByCertID(ctx context.Context, certID string) (*CertInfo, error) {
	var ci CertInfo
	err := r.pool.QueryRow(ctx,
		`SELECT cert_id, resource_type, resource_id, created_at::text
		 FROM certificates WHERE cert_id=$1`, certID).
		Scan(&ci.CertID, &ci.ResourceType, &ci.ResourceID, &ci.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find cert: %w", err)
	}
	return &ci, nil
}

func (r *pgRepo) Register(ctx context.Context, certID, resourceType, resourceID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO certificates (cert_id, resource_type, resource_id)
		 VALUES ($1,$2,$3) ON CONFLICT (cert_id) DO NOTHING`,
		certID, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("register cert: %w", err)
	}
	return nil
}
