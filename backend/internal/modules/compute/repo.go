package compute

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository abstracts compute persistence.
type Repository interface {
	// Algorithms
	CreateAlgo(ctx context.Context, a Algo) (Algo, error)
	GetAlgo(ctx context.Context, id string) (Algo, error)
	ListAlgosBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Algo, error)
	ListCurrentAlgos(ctx context.Context) ([]Algo, error)
	SetAlgoCurrentVersion(ctx context.Context, id string, current bool) error

	// Jobs
	CreateJob(ctx context.Context, j Job) (Job, error)
	GetJob(ctx context.Context, id string) (Job, error)
	ListJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Job, error)
	ListPendingJobs(ctx context.Context, limit int) ([]Job, error) // for worker loop
	UpdateJobStatus(ctx context.Context, id, status, errMsg string) error
	SetJobAttestation(ctx context.Context, id, inputHash, outputHash, signature string) error
	SetJobOutput(ctx context.Context, id string, outputKind string, outputBytes int64) error
}

// PostgresRepo implements Repository against PostgreSQL.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresRepo creates a Postgres-backed Repository.
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

// ── Algorithms ─────────────────────────────────────────────

func (r *PostgresRepo) CreateAlgo(ctx context.Context, a Algo) (Algo, error) {
	const q = `INSERT INTO compute_algorithms (id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q, a.ID, a.SellerID, a.Name, a.Runtime, a.Image, a.ImageDigest, a.Version, a.SourceRef, a.Entrypoint, a.OutputKind, a.ParamsSchema)
	return scanAlgo(row)
}

func (r *PostgresRepo) GetAlgo(ctx context.Context, id string) (Algo, error) {
	const q = `SELECT id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema, current_version, created_at, updated_at
		FROM compute_algorithms WHERE id = $1`
	return scanAlgo(r.pool.QueryRow(ctx, q, id))
}

func (r *PostgresRepo) ListAlgosBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Algo, error) {
	const q = `SELECT id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema, current_version, created_at, updated_at
		FROM compute_algorithms WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlgos(rows)
}

func (r *PostgresRepo) ListCurrentAlgos(ctx context.Context) ([]Algo, error) {
	const q = `SELECT id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema, current_version, created_at, updated_at
		FROM compute_algorithms WHERE current_version = true ORDER BY name`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlgos(rows)
}

func (r *PostgresRepo) SetAlgoCurrentVersion(ctx context.Context, id string, current bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE compute_algorithms SET current_version = $2, updated_at = NOW() WHERE id = $1`, id, current)
	return err
}

// ── Jobs ───────────────────────────────────────────────────

func (r *PostgresRepo) CreateJob(ctx context.Context, j Job) (Job, error) {
	const q = `INSERT INTO compute_jobs (id, algorithm_id, buyer_id, dataset_id, params, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, algorithm_id, buyer_id, dataset_id, params, status, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q, j.ID, j.AlgorithmID, j.BuyerID, j.DatasetID, j.Params, j.Status)
	return scanJob(row)
}

func (r *PostgresRepo) GetJob(ctx context.Context, id string) (Job, error) {
	const q = `SELECT id, algorithm_id, buyer_id, dataset_id, params, status, output_kind, output_bytes, error, attest_input_hash, attest_output_hash, attest_signature, attest_signed_at, created_at, updated_at
		FROM compute_jobs WHERE id = $1`
	return scanJob(r.pool.QueryRow(ctx, q, id))
}

func (r *PostgresRepo) ListJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Job, error) {
	const q = `SELECT id, algorithm_id, buyer_id, dataset_id, params, status, output_kind, output_bytes, error, attest_input_hash, attest_output_hash, attest_signature, attest_signed_at, created_at, updated_at
		FROM compute_jobs WHERE buyer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, buyerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (r *PostgresRepo) UpdateJobStatus(ctx context.Context, id, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `UPDATE compute_jobs SET status = $2, error = $3, updated_at = NOW() WHERE id = $1`, id, status, errMsg)
	return err
}

func (r *PostgresRepo) SetJobAttestation(ctx context.Context, id, inputHash, outputHash, signature string) error {
	_, err := r.pool.Exec(ctx, `UPDATE compute_jobs SET attest_input_hash = $2, attest_output_hash = $3, attest_signature = $4, attest_signed_at = NOW(), updated_at = NOW() WHERE id = $1`, id, inputHash, outputHash, signature)
	return err
}

func (r *PostgresRepo) SetJobOutput(ctx context.Context, id string, outputKind string, outputBytes int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE compute_jobs SET output_kind = $2, output_bytes = $3, updated_at = NOW() WHERE id = $1`, id, outputKind, outputBytes)
	return err
}

// ── Scanners ───────────────────────────────────────────────

func scanAlgo(row pgx.Row) (Algo, error) {
	var a Algo
	err := row.Scan(&a.ID, &a.SellerID, &a.Name, &a.Runtime, &a.Image, &a.ImageDigest,
		&a.Version, &a.SourceRef, &a.Entrypoint, &a.OutputKind, &a.ParamsSchema,
		&a.CurrentVersion, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func scanAlgos(rows pgx.Rows) ([]Algo, error) {
	var out []Algo
	for rows.Next() {
		var a Algo
		if err := rows.Scan(&a.ID, &a.SellerID, &a.Name, &a.Runtime, &a.Image, &a.ImageDigest,
			&a.Version, &a.SourceRef, &a.Entrypoint, &a.OutputKind, &a.ParamsSchema,
			&a.CurrentVersion, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *PostgresRepo) ListPendingJobs(ctx context.Context, limit int) ([]Job, error) {
	const q = `SELECT id, algorithm_id, buyer_id, dataset_id, params, status, output_kind, output_bytes, error, attest_input_hash, attest_output_hash, attest_signature, attest_signed_at, created_at, updated_at
		FROM compute_jobs WHERE status = 'pending' ORDER BY created_at ASC LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.AlgorithmID, &j.BuyerID, &j.DatasetID, &j.Params, &j.Status,
		&j.OutputKind, &j.OutputBytes, &j.Error,
		&j.AttestInputHash, &j.AttestOutputHash, &j.AttestSignature, &j.AttestSignedAt,
		&j.CreatedAt, &j.UpdatedAt)
	return j, err
}

func scanJobs(rows pgx.Rows) ([]Job, error) {
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.AlgorithmID, &j.BuyerID, &j.DatasetID, &j.Params, &j.Status,
			&j.OutputKind, &j.OutputBytes, &j.Error,
			&j.AttestInputHash, &j.AttestOutputHash, &j.AttestSignature, &j.AttestSignedAt,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// ── Helpers ────────────────────────────────────────────────

func tableColumns(table string) string {
	switch table {
	case "compute_algorithms":
		return "id, seller_id, name, runtime, image, image_digest, version, source_ref, entrypoint, output_kind, params_schema, current_version, created_at, updated_at"
	case "compute_jobs":
		return "id, algorithm_id, buyer_id, dataset_id, params, status, output_kind, output_bytes, error, attest_input_hash, attest_output_hash, attest_signature, attest_signed_at, created_at, updated_at"
	default:
		return ""
	}
}

var _ = tableColumns // used in service layer reflection
