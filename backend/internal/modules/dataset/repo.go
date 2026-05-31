package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository abstracts dataset persistence (owns dataset/dataset_version/
// dataset_file). Service logic is unit-tested against an in-memory fake.
type Repository interface {
	Create(ctx context.Context, d Dataset) (Dataset, error)
	GetByID(ctx context.Context, id string) (Dataset, error)
	UpdateMeta(ctx context.Context, d Dataset) (Dataset, error)
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Dataset, error)
	SignSource(ctx context.Context, id string) (Dataset, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const datasetCols = `id, seller_id, title, description, data_type, COALESCE(domain,''),
	license_type, suggested_price_cents, final_price_cents, status,
	total_size_bytes, sample_count, source_declaration,
	source_signed_at::text, COALESCE(current_version_id::text,''),
	created_at::text, updated_at::text`

func scanDataset(row pgx.Row) (Dataset, error) {
	var d Dataset
	var decl []byte
	var signedAt *string
	if err := row.Scan(
		&d.ID, &d.SellerID, &d.Title, &d.Description, &d.DataType, &d.Domain,
		&d.LicenseType, &d.SuggestedPriceCents, &d.FinalPriceCents, &d.Status,
		&d.TotalSizeBytes, &d.SampleCount, &decl,
		&signedAt, &d.CurrentVersionID, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return Dataset{}, err
	}
	if signedAt != nil {
		d.SourceSignedAt = *signedAt
	}
	if len(decl) > 0 {
		_ = json.Unmarshal(decl, &d.SourceDeclaration)
	}
	return d, nil
}

func (r *pgRepo) Create(ctx context.Context, d Dataset) (Dataset, error) {
	decl, _ := json.Marshal(d.SourceDeclaration)
	const q = `
		INSERT INTO datasets (seller_id, title, description, data_type, domain,
			license_type, suggested_price_cents, status, source_declaration)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,'draft',$8::jsonb)
		RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q,
		d.SellerID, d.Title, d.Description, d.DataType, d.Domain,
		d.LicenseType, d.SuggestedPriceCents, string(decl)))
	if err != nil {
		return Dataset{}, fmt.Errorf("create dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetByID(ctx context.Context, id string) (Dataset, error) {
	out, err := scanDataset(r.pool.QueryRow(ctx, `SELECT `+datasetCols+` FROM datasets WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("get dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) UpdateMeta(ctx context.Context, d Dataset) (Dataset, error) {
	decl, _ := json.Marshal(d.SourceDeclaration)
	const q = `
		UPDATE datasets SET title=$2, description=$3, data_type=$4, domain=NULLIF($5,''),
			license_type=$6, suggested_price_cents=$7, source_declaration=$8::jsonb, updated_at=now()
		WHERE id=$1
		RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q,
		d.ID, d.Title, d.Description, d.DataType, d.Domain,
		d.LicenseType, d.SuggestedPriceCents, string(decl)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("update dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Dataset, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+datasetCols+` FROM datasets WHERE seller_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		sellerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgRepo) SignSource(ctx context.Context, id string) (Dataset, error) {
	const q = `UPDATE datasets SET source_signed_at=now(), updated_at=now() WHERE id=$1 RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("sign source: %w", err)
	}
	return out, nil
}
