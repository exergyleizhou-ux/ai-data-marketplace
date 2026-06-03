package compute

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// fedCols is the column list / scan order for compute_federated_jobs.
const fedCols = `id, buyer_id, COALESCE(algorithm_id::text,''), dataset_ids::text[], mode, status,
	min_participants, params, dp_epsilon, COALESCE(output_key,''), output_bytes,
	COALESCE(failure_code,''), created_at::text, updated_at::text`

func scanFed(row pgx.Row) (FederatedJob, error) {
	var f FederatedJob
	var params []byte
	var eps sql.NullFloat64
	err := row.Scan(&f.ID, &f.BuyerID, &f.AlgorithmID, &f.DatasetIDs, &f.Mode, &f.Status,
		&f.MinParticipants, &params, &eps, &f.OutputKey, &f.OutputBytes,
		&f.FailureCode, &f.CreatedAt, &f.UpdatedAt)
	f.Params = fromJSONB(params)
	f.DPEpsilon = ptrF(eps)
	return f, err
}

func (r *pgRepo) CreateFederatedJob(ctx context.Context, f FederatedJob) (FederatedJob, error) {
	if f.Mode == "" {
		f.Mode = ModeFederated
	}
	params, err := toJSONB(f.Params)
	if err != nil {
		return FederatedJob{}, fmt.Errorf("marshal federated params: %w", err)
	}
	if params == nil {
		params = []byte("{}") // column is NOT NULL DEFAULT '{}'; explicit NULL would violate it
	}
	const q = `
		INSERT INTO compute_federated_jobs (buyer_id, algorithm_id, dataset_ids, mode, status,
			min_participants, params, dp_epsilon)
		VALUES ($1, NULLIF($2,'')::uuid, $3::uuid[], $4, $5, $6, $7, $8)
		RETURNING ` + fedCols
	out, err := scanFed(r.pool.QueryRow(ctx, q,
		f.BuyerID, f.AlgorithmID, f.DatasetIDs, f.Mode, FedCreated,
		f.MinParticipants, params, nullF(f.DPEpsilon)))
	if err != nil {
		return FederatedJob{}, fmt.Errorf("create federated job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetFederatedJob(ctx context.Context, id string) (FederatedJob, error) {
	out, err := scanFed(r.pool.QueryRow(ctx, `SELECT `+fedCols+` FROM compute_federated_jobs WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return FederatedJob{}, ErrNotFound
	}
	if err != nil {
		return FederatedJob{}, fmt.Errorf("get federated job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListSubJobs(ctx context.Context, federatedID string) ([]Job, error) {
	return r.listJobs(ctx,
		`SELECT `+jobCols+` FROM compute_jobs WHERE federated_job_id=$1 ORDER BY created_at, id`,
		federatedID)
}

func (r *pgRepo) TransitionFederated(ctx context.Context, id, from, to string) (FederatedJob, error) {
	out, err := scanFed(r.pool.QueryRow(ctx,
		`UPDATE compute_federated_jobs SET status=$3, updated_at=now() WHERE id=$1 AND status=$2 RETURNING `+fedCols,
		id, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		return FederatedJob{}, ErrBadTransition
	}
	if err != nil {
		return FederatedJob{}, fmt.Errorf("transition federated: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ReleaseFederated(ctx context.Context, id, outputKey string, outputBytes int64) (FederatedJob, error) {
	out, err := scanFed(r.pool.QueryRow(ctx,
		`UPDATE compute_federated_jobs SET status=$2, output_key=$3, output_bytes=$4, updated_at=now()
		 WHERE id=$1 AND status=$5 RETURNING `+fedCols,
		id, FedReleased, outputKey, outputBytes, FedAggregating))
	if errors.Is(err, pgx.ErrNoRows) {
		return FederatedJob{}, ErrBadTransition
	}
	if err != nil {
		return FederatedJob{}, fmt.Errorf("release federated: %w", err)
	}
	return out, nil
}

func (r *pgRepo) FailFederated(ctx context.Context, id, code string) (FederatedJob, error) {
	// Idempotent fail: allow from any non-terminal status; no-op (ErrBadTransition)
	// if already terminal, so concurrent sub-job failures don't double-fail.
	out, err := scanFed(r.pool.QueryRow(ctx,
		`UPDATE compute_federated_jobs SET status=$2, failure_code=$3, updated_at=now()
		 WHERE id=$1 AND status NOT IN ($4,$5,$6) RETURNING `+fedCols,
		id, FedFailed, code, FedReleased, FedFailed, FedRejected))
	if errors.Is(err, pgx.ErrNoRows) {
		return FederatedJob{}, ErrBadTransition
	}
	if err != nil {
		return FederatedJob{}, fmt.Errorf("fail federated: %w", err)
	}
	return out, nil
}
