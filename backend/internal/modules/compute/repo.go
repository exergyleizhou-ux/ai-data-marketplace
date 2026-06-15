package compute

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the compute tables (algorithms, dataset_compute_offers,
// compute_entitlements, compute_jobs, dp_budget_ledger). Status changes go
// through optimistic UPDATEs (WHERE status=from) so the job state machine and
// the quota/lease invariants are enforced at the DB level too — the same
// approach the order module uses for orders.
type Repository interface {
	// algorithms
	RegisterAlgorithm(ctx context.Context, a Algorithm) (Algorithm, error)
	GetAlgorithm(ctx context.Context, id string) (Algorithm, error)
	ListApprovedAlgorithms(ctx context.Context) ([]Algorithm, error)
	ListAlgorithmsByStatus(ctx context.Context, status string, limit int) ([]Algorithm, error)
	ListAlgorithmsByOwner(ctx context.Context, ownerID string) ([]Algorithm, error)
	ReviewAlgorithm(ctx context.Context, id, status string, trusted bool) (Algorithm, error)

	// offers
	UpsertOffer(ctx context.Context, o Offer) (Offer, error)
	GetOffer(ctx context.Context, datasetID string) (Offer, error)

	// entitlements
	CreateEntitlement(ctx context.Context, e Entitlement) (Entitlement, error)
	GetEntitlement(ctx context.Context, id string) (Entitlement, error)
	ListEntitlementsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Entitlement, error)
	// SpendQuota atomically consumes one job credit: it only succeeds when the
	// entitlement is active and jobs_used < jobs_quota, marking it exhausted on
	// the last credit. Concurrency-safe (single conditional UPDATE) so parallel
	// submits cannot over-spend. Returns ErrQuotaExhausted / ErrEntitlementState.
	SpendQuota(ctx context.Context, entitlementID string) (Entitlement, error)
	// RefundQuota gives a credit back (used when a job fails for a platform-side
	// reason and should not be billed — design §21). Re-activates if exhausted.
	RefundQuota(ctx context.Context, entitlementID string) error
	// RevokeByOrder revokes all entitlements tied to an order (H2 refund
	// linkage); returns the number revoked.
	RevokeByOrder(ctx context.Context, orderID string) (int, error)

	// jobs
	CreateJob(ctx context.Context, j Job) (Job, error)
	GetJob(ctx context.Context, id string) (Job, error)
	GetJobByIdempotency(ctx context.Context, entitlementID, key string) (Job, error)
	ListJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Job, error)
	ListJobsByStatus(ctx context.Context, status string, limit int) ([]Job, error)
	// Transition moves a job from->to atomically; ErrBadTransition if the
	// current status is not `from`.
	Transition(ctx context.Context, id, from, to string) (Job, error)
	// ClaimJob atomically takes a queued job for `runnerID`, setting status=running,
	// the lease, started_at (first attempt) and bumping attempts. ErrBadTransition
	// if the job is not queued.
	ClaimJob(ctx context.Context, id, runnerID string, leaseSecs int) (Job, error)
	// Heartbeat extends the lease for a running job still owned by runnerID.
	Heartbeat(ctx context.Context, id, runnerID string, leaseSecs int) error
	// Release marks a job released with its output (running/output_pending ->
	// released). Idempotent: a job already released returns without error so a
	// retried runner cannot double-release.
	Release(ctx context.Context, id, outputKey, outputKind string, outputBytes int64, logsKey string) (Job, error)
	// StageForReview stores a job's output and parks it in output_reviewing
	// (running -> output_reviewing) for ops human review before release — used
	// when the offer sets review_output (high-sensitivity datasets, §8 gate ⑤).
	StageForReview(ctx context.Context, id, outputKey, outputKind string, outputBytes int64, logsKey string) (Job, error)
	// Fail marks a job failed with a de-identified error (never raw stdout).
	Fail(ctx context.Context, id, errCode string) (Job, error)
	// Reject marks a job's output rejected by the output gate (size/DP/leak/
	// human review) — distinct from Fail (execution error); billing differs (§21).
	Reject(ctx context.Context, id, reason string) (Job, error)
	// SetAttestation stores a job's L2 TEE remote-attestation report (design P3).
	SetAttestation(ctx context.Context, id string, report []byte) error
	// ReclaimStaleLeases requeues running jobs whose lease expired (runner
	// presumed crashed): attempts < maxAttempts -> queued, else -> failed.
	// Returns the number of jobs reclaimed. Used by the recovery sweep (§17).
	ReclaimStaleLeases(ctx context.Context, maxAttempts int) (int, error)

	// differential-privacy budget
	SpendDP(ctx context.Context, datasetID, buyerID, jobID string, eps float64) error
	SumDP(ctx context.Context, datasetID, buyerID string) (float64, error)

	// federated (P4-a)
	CreateFederatedJob(ctx context.Context, f FederatedJob) (FederatedJob, error)
	GetFederatedJob(ctx context.Context, id string) (FederatedJob, error)
	ListFederatedJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]FederatedJob, error)
	ListSubJobs(ctx context.Context, federatedID string) ([]Job, error)
	TransitionFederated(ctx context.Context, id, from, to string) (FederatedJob, error)
	ReleaseFederated(ctx context.Context, id, outputKey string, outputBytes int64) (FederatedJob, error)
	FailFederated(ctx context.Context, id, code string) (FederatedJob, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

// NewRepository returns a Postgres-backed compute Repository.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const uniqueViolation = "23505"

// --- jsonb helpers ---

func toJSONB(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func fromJSONB(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

func nullF(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func ptrF(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// --- algorithms ---

const algoCols = `id, COALESCE(owner_id::text,''), name, runtime, image, image_digest,
	version, source_ref, entrypoint, params_schema, output_kind, status, trusted,
	created_at::text, updated_at::text`

func scanAlgo(row pgx.Row) (Algorithm, error) {
	var a Algorithm
	var schema []byte
	err := row.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Runtime, &a.Image, &a.ImageDigest,
		&a.Version, &a.SourceRef, &a.Entrypoint, &schema, &a.OutputKind, &a.Status, &a.Trusted,
		&a.CreatedAt, &a.UpdatedAt)
	a.ParamsSchema = fromJSONB(schema)
	return a, err
}

func (r *pgRepo) RegisterAlgorithm(ctx context.Context, a Algorithm) (Algorithm, error) {
	schema, err := toJSONB(a.ParamsSchema)
	if err != nil {
		return Algorithm{}, fmt.Errorf("marshal params_schema: %w", err)
	}
	if a.Version == 0 {
		a.Version = 1
	}
	const q = `
		INSERT INTO algorithms (owner_id, name, runtime, image, image_digest, version,
			source_ref, entrypoint, params_schema, output_kind, status, trusted)
		VALUES (NULLIF($1,'')::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING ` + algoCols
	status := a.Status
	if status == "" {
		status = AlgoPending
	}
	out, err := scanAlgo(r.pool.QueryRow(ctx, q,
		a.OwnerID, a.Name, a.Runtime, a.Image, a.ImageDigest, a.Version,
		a.SourceRef, a.Entrypoint, schema, a.OutputKind, status, a.Trusted))
	if err != nil {
		return Algorithm{}, fmt.Errorf("register algorithm: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetAlgorithm(ctx context.Context, id string) (Algorithm, error) {
	out, err := scanAlgo(r.pool.QueryRow(ctx, `SELECT `+algoCols+` FROM algorithms WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Algorithm{}, ErrNotFound
	}
	if err != nil {
		return Algorithm{}, fmt.Errorf("get algorithm: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListApprovedAlgorithms(ctx context.Context) ([]Algorithm, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+algoCols+` FROM algorithms WHERE status=$1 ORDER BY name`, AlgoApproved)
	if err != nil {
		return nil, fmt.Errorf("list algorithms: %w", err)
	}
	defer rows.Close()
	var out []Algorithm
	for rows.Next() {
		a, err := scanAlgo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAlgorithmsByOwner returns the algorithms a user submitted (any status),
// newest first, so requesters can track review progress.
func (r *pgRepo) ListAlgorithmsByOwner(ctx context.Context, ownerID string) ([]Algorithm, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+algoCols+` FROM algorithms WHERE owner_id=$1::uuid ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list algorithms by owner: %w", err)
	}
	defer rows.Close()
	var out []Algorithm
	for rows.Next() {
		a, err := scanAlgo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *pgRepo) ListAlgorithmsByStatus(ctx context.Context, status string, limit int) ([]Algorithm, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT `+algoCols+` FROM algorithms WHERE status=$1 ORDER BY updated_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list algorithms by status: %w", err)
	}
	defer rows.Close()
	var out []Algorithm
	for rows.Next() {
		a, err := scanAlgo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *pgRepo) ReviewAlgorithm(ctx context.Context, id, status string, trusted bool) (Algorithm, error) {
	out, err := scanAlgo(r.pool.QueryRow(ctx,
		`UPDATE algorithms SET status=$2, trusted=$3, updated_at=now() WHERE id=$1 RETURNING `+algoCols,
		id, status, trusted))
	if errors.Is(err, pgx.ErrNoRows) {
		return Algorithm{}, ErrNotFound
	}
	if err != nil {
		return Algorithm{}, fmt.Errorf("review algorithm: %w", err)
	}
	return out, nil
}

// --- offers ---

const offerCols = `dataset_id, enabled, allow_custom, allowed_algorithm_ids::text[],
	price_cents, max_runtime_secs, max_output_bytes, max_output_files,
	dp_epsilon, dp_epsilon_total, return_logs, review_output, trust_level, allow_federated, allow_psi, updated_at::text`

func scanOffer(row pgx.Row) (Offer, error) {
	var o Offer
	var eps, epsTotal sql.NullFloat64
	err := row.Scan(&o.DatasetID, &o.Enabled, &o.AllowCustom, &o.AllowedAlgoIDs,
		&o.PriceCents, &o.MaxRuntimeSecs, &o.MaxOutputBytes, &o.MaxOutputFiles,
		&eps, &epsTotal, &o.ReturnLogs, &o.ReviewOutput, &o.TrustLevel, &o.AllowFederated, &o.AllowPSI, &o.UpdatedAt)
	o.DPEpsilon = ptrF(eps)
	o.DPEpsilonTotal = ptrF(epsTotal)
	if o.AllowedAlgoIDs == nil {
		o.AllowedAlgoIDs = []string{}
	}
	return o, err
}

func (r *pgRepo) UpsertOffer(ctx context.Context, o Offer) (Offer, error) {
	if o.TrustLevel == "" {
		o.TrustLevel = TrustL1
	}
	if o.AllowedAlgoIDs == nil {
		o.AllowedAlgoIDs = []string{}
	}
	const q = `
		INSERT INTO dataset_compute_offers (dataset_id, enabled, allow_custom, allowed_algorithm_ids,
			price_cents, max_runtime_secs, max_output_bytes, max_output_files,
			dp_epsilon, dp_epsilon_total, return_logs, review_output, trust_level, allow_federated, allow_psi, updated_at)
		VALUES ($1,$2,$3,$4::uuid[],$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15, now())
		ON CONFLICT (dataset_id) DO UPDATE SET
			enabled=EXCLUDED.enabled, allow_custom=EXCLUDED.allow_custom,
			allowed_algorithm_ids=EXCLUDED.allowed_algorithm_ids, price_cents=EXCLUDED.price_cents,
			max_runtime_secs=EXCLUDED.max_runtime_secs, max_output_bytes=EXCLUDED.max_output_bytes,
			max_output_files=EXCLUDED.max_output_files, dp_epsilon=EXCLUDED.dp_epsilon,
			dp_epsilon_total=EXCLUDED.dp_epsilon_total, return_logs=EXCLUDED.return_logs,
			review_output=EXCLUDED.review_output, trust_level=EXCLUDED.trust_level,
			allow_federated=EXCLUDED.allow_federated, allow_psi=EXCLUDED.allow_psi, updated_at=now()
		RETURNING ` + offerCols
	out, err := scanOffer(r.pool.QueryRow(ctx, q,
		o.DatasetID, o.Enabled, o.AllowCustom, o.AllowedAlgoIDs,
		o.PriceCents, o.MaxRuntimeSecs, o.MaxOutputBytes, o.MaxOutputFiles,
		nullF(o.DPEpsilon), nullF(o.DPEpsilonTotal), o.ReturnLogs, o.ReviewOutput, o.TrustLevel, o.AllowFederated, o.AllowPSI))
	if err != nil {
		return Offer{}, fmt.Errorf("upsert offer: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetOffer(ctx context.Context, datasetID string) (Offer, error) {
	out, err := scanOffer(r.pool.QueryRow(ctx, `SELECT `+offerCols+` FROM dataset_compute_offers WHERE dataset_id=$1`, datasetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, ErrNotFound
	}
	if err != nil {
		return Offer{}, fmt.Errorf("get offer: %w", err)
	}
	return out, nil
}

// --- entitlements ---

const entCols = `id, dataset_id, buyer_id, COALESCE(order_id::text,''), jobs_quota, jobs_used,
	status, COALESCE(expires_at::text,''), created_at::text`

func scanEnt(row pgx.Row) (Entitlement, error) {
	var e Entitlement
	err := row.Scan(&e.ID, &e.DatasetID, &e.BuyerID, &e.OrderID, &e.JobsQuota, &e.JobsUsed,
		&e.Status, &e.ExpiresAt, &e.CreatedAt)
	return e, err
}

func (r *pgRepo) CreateEntitlement(ctx context.Context, e Entitlement) (Entitlement, error) {
	if e.JobsQuota < 1 {
		e.JobsQuota = 1
	}
	const q = `
		INSERT INTO compute_entitlements (dataset_id, buyer_id, order_id, jobs_quota, expires_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,$4,NULLIF($5,'')::timestamptz)
		RETURNING ` + entCols
	out, err := scanEnt(r.pool.QueryRow(ctx, q, e.DatasetID, e.BuyerID, e.OrderID, e.JobsQuota, e.ExpiresAt))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Entitlement{}, ErrDuplicateEnt // one entitlement per order (idempotent grant)
		}
		return Entitlement{}, fmt.Errorf("create entitlement: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetEntitlement(ctx context.Context, id string) (Entitlement, error) {
	out, err := scanEnt(r.pool.QueryRow(ctx, `SELECT `+entCols+` FROM compute_entitlements WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Entitlement{}, ErrNotFound
	}
	if err != nil {
		return Entitlement{}, fmt.Errorf("get entitlement: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListEntitlementsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Entitlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+entCols+` FROM compute_entitlements WHERE buyer_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		buyerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list entitlements: %w", err)
	}
	defer rows.Close()
	var out []Entitlement
	for rows.Next() {
		e, err := scanEnt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *pgRepo) SpendQuota(ctx context.Context, id string) (Entitlement, error) {
	// Atomic single-statement spend: only when active, not expired, and credits
	// remain. Flip to exhausted on the last credit. Concurrency-safe.
	const q = `
		UPDATE compute_entitlements
		SET jobs_used = jobs_used + 1,
		    status = CASE WHEN jobs_used + 1 >= jobs_quota THEN 'exhausted' ELSE status END
		WHERE id=$1 AND status='active' AND jobs_used < jobs_quota
		  AND (expires_at IS NULL OR expires_at > now())
		RETURNING ` + entCols
	out, err := scanEnt(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		// The atomic update didn't fire. Distinguish the precise reason:
		//   revoked/expired-status -> ErrEntitlementState
		//   out of credits (exhausted or full) -> ErrQuotaExhausted
		//   otherwise (e.g. time-expired active) -> ErrEntitlementState
		e, gerr := r.GetEntitlement(ctx, id)
		if gerr != nil {
			return Entitlement{}, gerr
		}
		switch {
		case e.Status == EntRevoked || e.Status == EntExpired:
			return Entitlement{}, ErrEntitlementState
		case e.JobsUsed >= e.JobsQuota:
			return Entitlement{}, ErrQuotaExhausted
		default:
			return Entitlement{}, ErrEntitlementState
		}
	}
	if err != nil {
		return Entitlement{}, fmt.Errorf("spend quota: %w", err)
	}
	return out, nil
}

func (r *pgRepo) RefundQuota(ctx context.Context, id string) error {
	const q = `
		UPDATE compute_entitlements
		SET jobs_used = GREATEST(jobs_used - 1, 0),
		    status = CASE WHEN status='exhausted' THEN 'active' ELSE status END
		WHERE id=$1 AND status IN ('active','exhausted')`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("refund quota: %w", err)
	}
	return nil
}

func (r *pgRepo) RevokeByOrder(ctx context.Context, orderID string) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE compute_entitlements SET status='revoked' WHERE order_id=$1 AND status IN ('active','exhausted')`,
		orderID)
	if err != nil {
		return 0, fmt.Errorf("revoke by order: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// --- jobs ---

const jobCols = `id, dataset_id, COALESCE(version_id::text,''), buyer_id, entitlement_id,
	COALESCE(algorithm_id::text,''), COALESCE(algorithm_version,0), params, status, attempts,
	dp_epsilon, COALESCE(output_key,''), COALESCE(output_bytes,0), COALESCE(output_kind,''),
	COALESCE(logs_key,''), COALESCE(error,''), attestation, COALESCE(federated_job_id::text,''), created_at::text,
	COALESCE(started_at::text,''), COALESCE(finished_at::text,'')`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	var params, attestation []byte
	var eps sql.NullFloat64
	err := row.Scan(&j.ID, &j.DatasetID, &j.VersionID, &j.BuyerID, &j.EntitlementID,
		&j.AlgorithmID, &j.AlgorithmVersion, &params, &j.Status, &j.Attempts,
		&eps, &j.OutputKey, &j.OutputBytes, &j.OutputKind,
		&j.LogsKey, &j.Error, &attestation, &j.FederatedJobID, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	j.Params = fromJSONB(params)
	j.Attestation = fromJSONB(attestation)
	j.DPEpsilon = ptrF(eps)
	return j, err
}

// SetAttestation stores a job's L2 remote-attestation report (design P3).
func (r *pgRepo) SetAttestation(ctx context.Context, id string, report []byte) error {
	_, err := r.pool.Exec(ctx, `UPDATE compute_jobs SET attestation=$2 WHERE id=$1`, id, report)
	if err != nil {
		return fmt.Errorf("set attestation: %w", err)
	}
	return nil
}

func (r *pgRepo) CreateJob(ctx context.Context, j Job) (Job, error) {
	params, err := toJSONB(j.Params)
	if err != nil {
		return Job{}, fmt.Errorf("marshal params: %w", err)
	}
	const q = `
		INSERT INTO compute_jobs (dataset_id, version_id, buyer_id, entitlement_id,
			algorithm_id, algorithm_version, params, idempotency_key, status, dp_epsilon, federated_job_id)
		VALUES ($1, NULLIF($2,'')::uuid, $3, $4, NULLIF($5,'')::uuid, NULLIF($6,0), $7,
			NULLIF($8,''), $9, $10, NULLIF($11,'')::uuid)
		RETURNING ` + jobCols
	status := j.Status
	if status == "" {
		status = JobCreated
	}
	out, err := scanJob(r.pool.QueryRow(ctx, q,
		j.DatasetID, j.VersionID, j.BuyerID, j.EntitlementID,
		j.AlgorithmID, j.AlgorithmVersion, params, j.idemKey, status, nullF(j.DPEpsilon), j.FederatedJobID))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			// Idempotent submit collision: the service refunds the quota it spent
			// and returns the pre-existing job (via GetJobByIdempotency).
			return Job{}, ErrDuplicateJob
		}
		return Job{}, fmt.Errorf("create job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetJobByIdempotency(ctx context.Context, entitlementID, key string) (Job, error) {
	out, err := scanJob(r.pool.QueryRow(ctx,
		`SELECT `+jobCols+` FROM compute_jobs WHERE entitlement_id=$1 AND idempotency_key=$2`, entitlementID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return out, err
}

func (r *pgRepo) GetJob(ctx context.Context, id string) (Job, error) {
	out, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobCols+` FROM compute_jobs WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Job, error) {
	return r.listJobs(ctx,
		`SELECT `+jobCols+` FROM compute_jobs WHERE buyer_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		buyerID, limit, offset)
}

func (r *pgRepo) ListJobsByStatus(ctx context.Context, status string, limit int) ([]Job, error) {
	return r.listJobs(ctx,
		`SELECT `+jobCols+` FROM compute_jobs WHERE status=$1 ORDER BY created_at LIMIT $2`,
		status, limit)
}

func (r *pgRepo) listJobs(ctx context.Context, q string, args ...any) ([]Job, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *pgRepo) Transition(ctx context.Context, id, from, to string) (Job, error) {
	out, err := scanJob(r.pool.QueryRow(ctx,
		`UPDATE compute_jobs SET status=$3 WHERE id=$1 AND status=$2 RETURNING `+jobCols, id, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("transition job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ClaimJob(ctx context.Context, id, runnerID string, leaseSecs int) (Job, error) {
	const q = `
		UPDATE compute_jobs
		SET status='running', runner_id=$2,
		    lease_until = now() + $3 * interval '1 second',
		    attempts = attempts + 1,
		    started_at = COALESCE(started_at, now())
		WHERE id=$1 AND status='queued'
		RETURNING ` + jobCols
	out, err := scanJob(r.pool.QueryRow(ctx, q, id, runnerID, leaseSecs))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("claim job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) Heartbeat(ctx context.Context, id, runnerID string, leaseSecs int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE compute_jobs SET lease_until = now() + $3 * interval '1 second'
		 WHERE id=$1 AND runner_id=$2 AND status='running'`, id, runnerID, leaseSecs)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBadTransition
	}
	return nil
}

func (r *pgRepo) Release(ctx context.Context, id, outputKey, outputKind string, outputBytes int64, logsKey string) (Job, error) {
	// Idempotent: if already released, return it unchanged (a retried runner must
	// not double-release / double-bill — design §17.1).
	if cur, err := r.GetJob(ctx, id); err == nil && cur.Status == JobReleased {
		return cur, nil
	}
	const q = `
		UPDATE compute_jobs
		SET status='released', output_key=$2, output_kind=$3, output_bytes=$4,
		    logs_key=NULLIF($5,''), lease_until=NULL, finished_at=now()
		WHERE id=$1 AND status IN ('running','output_pending','output_reviewing')
		RETURNING ` + jobCols
	out, err := scanJob(r.pool.QueryRow(ctx, q, id, outputKey, outputKind, outputBytes, logsKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("release job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) StageForReview(ctx context.Context, id, outputKey, outputKind string, outputBytes int64, logsKey string) (Job, error) {
	const q = `
		UPDATE compute_jobs
		SET status='output_reviewing', output_key=$2, output_kind=$3, output_bytes=$4,
		    logs_key=NULLIF($5,''), lease_until=NULL
		WHERE id=$1 AND status IN ('running','output_pending')
		RETURNING ` + jobCols
	out, err := scanJob(r.pool.QueryRow(ctx, q, id, outputKey, outputKind, outputBytes, logsKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("stage for review: %w", err)
	}
	return out, nil
}

func (r *pgRepo) Fail(ctx context.Context, id, errCode string) (Job, error) {
	const q = `
		UPDATE compute_jobs SET status='failed', error=$2, lease_until=NULL, finished_at=now()
		WHERE id=$1 AND status NOT IN ('released','rejected','canceled','failed')
		RETURNING ` + jobCols
	out, err := scanJob(r.pool.QueryRow(ctx, q, id, errCode))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("fail job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) Reject(ctx context.Context, id, reason string) (Job, error) {
	const q = `
		UPDATE compute_jobs SET status='rejected', error=$2, lease_until=NULL, finished_at=now()
		WHERE id=$1 AND status IN ('running','output_pending','output_reviewing')
		RETURNING ` + jobCols
	out, err := scanJob(r.pool.QueryRow(ctx, q, id, reason))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrBadTransition
	}
	if err != nil {
		return Job{}, fmt.Errorf("reject job: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ReclaimStaleLeases(ctx context.Context, maxAttempts int) (int, error) {
	// Requeue crashed runs (lease expired) for retry; exhaust to failed once
	// attempts hit the cap.
	const q = `
		UPDATE compute_jobs
		SET status = CASE WHEN attempts < $1 THEN 'queued' ELSE 'failed' END,
		    runner_id = NULL, lease_until = NULL,
		    error = CASE WHEN attempts >= $1 THEN 'runner_crash_retries_exhausted' ELSE error END,
		    finished_at = CASE WHEN attempts >= $1 THEN now() ELSE finished_at END
		WHERE status='running' AND lease_until IS NOT NULL AND lease_until < now()`
	tag, err := r.pool.Exec(ctx, q, maxAttempts)
	if err != nil {
		return 0, fmt.Errorf("reclaim stale leases: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// --- DP budget ---

func (r *pgRepo) SpendDP(ctx context.Context, datasetID, buyerID, jobID string, eps float64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO dp_budget_ledger (dataset_id, buyer_id, job_id, epsilon_spent)
		 VALUES ($1,$2,NULLIF($3,'')::uuid,$4)`, datasetID, buyerID, jobID, eps)
	if err != nil {
		return fmt.Errorf("spend dp: %w", err)
	}
	return nil
}

func (r *pgRepo) SumDP(ctx context.Context, datasetID, buyerID string) (float64, error) {
	var sum float64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(epsilon_spent),0) FROM dp_budget_ledger WHERE dataset_id=$1 AND buyer_id=$2`,
		datasetID, buyerID).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum dp: %w", err)
	}
	return sum, nil
}
