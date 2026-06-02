package compute

import "errors"

// --- algorithms ---

// Algorithm is a registered computation a buyer can run inside the sandbox.
type Algorithm struct {
	ID           string         `json:"id"`
	OwnerID      string         `json:"owner_id,omitempty"` // "" = platform built-in
	Name         string         `json:"name"`
	Runtime      string         `json:"runtime"`
	Image        string         `json:"image"`
	ImageDigest  string         `json:"image_digest,omitempty"`
	Version      int            `json:"version"`
	SourceRef    string         `json:"source_ref,omitempty"`
	Entrypoint   string         `json:"entrypoint,omitempty"`
	ParamsSchema map[string]any `json:"params_schema,omitempty"`
	OutputKind   string         `json:"output_kind"`
	Status       string         `json:"status"`
	Trusted      bool           `json:"trusted"`
	CreatedAt    string         `json:"created_at,omitempty"`
	UpdatedAt    string         `json:"updated_at,omitempty"`
}

// Algorithm runtimes (P1 ships python-sklearn; others are roadmap).
const (
	RuntimeSklearn  = "python-sklearn"
	RuntimeLightGBM = "python-lightgbm"
	RuntimeSQL      = "sql"
	RuntimeCustom   = "custom-image"
)

// Algorithm output kinds.
const (
	OutputModel     = "model"
	OutputMetrics   = "metrics"
	OutputTable     = "table"
	OutputAggregate = "aggregate"
)

// Algorithm review statuses.
const (
	AlgoPending  = "pending"
	AlgoApproved = "approved"
	AlgoRejected = "rejected"
	AlgoDisabled = "disabled"
)

// --- offers ---

// Offer is a dataset's sandbox-sale configuration (coexists with the download
// product). The zero value (enabled=false) means the dataset is download-only.
type Offer struct {
	DatasetID      string   `json:"dataset_id"`
	Enabled        bool     `json:"enabled"`
	AllowCustom    bool     `json:"allow_custom"`
	AllowedAlgoIDs []string `json:"allowed_algorithm_ids"`
	PriceCents     int64    `json:"price_cents"`
	MaxRuntimeSecs int      `json:"max_runtime_secs"`
	MaxOutputBytes int64    `json:"max_output_bytes"`
	MaxOutputFiles int      `json:"max_output_files"`
	DPEpsilon      *float64 `json:"dp_epsilon,omitempty"`
	DPEpsilonTotal *float64 `json:"dp_epsilon_total,omitempty"`
	ReturnLogs     bool     `json:"return_logs"`
	ReviewOutput   bool     `json:"review_output"`
	TrustLevel     string   `json:"trust_level"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// Trust levels (design §2).
const (
	TrustL1 = "L1" // data sandbox: buyer-invisible, platform-visible
	TrustL2 = "L2" // confidential computing (TEE): platform-invisible
	TrustL3 = "L3" // data-stays-home (federated / MPC)
)

// --- entitlements ---

// Entitlement is a buyer's purchased compute credits on a dataset.
type Entitlement struct {
	ID        string `json:"id"`
	DatasetID string `json:"dataset_id"`
	BuyerID   string `json:"buyer_id"`
	OrderID   string `json:"order_id,omitempty"`
	JobsQuota int    `json:"jobs_quota"`
	JobsUsed  int    `json:"jobs_used"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Entitlement statuses.
const (
	EntActive    = "active"
	EntExhausted = "exhausted"
	EntExpired   = "expired"
	EntRevoked   = "revoked"
)

// --- jobs ---

// Job is one compute-to-data execution.
type Job struct {
	ID               string         `json:"id"`
	DatasetID        string         `json:"dataset_id"`
	VersionID        string         `json:"version_id,omitempty"`
	BuyerID          string         `json:"buyer_id"`
	EntitlementID    string         `json:"entitlement_id"`
	AlgorithmID      string         `json:"algorithm_id,omitempty"`
	AlgorithmVersion int            `json:"algorithm_version,omitempty"`
	Params           map[string]any `json:"params,omitempty"`
	Status           string         `json:"status"`
	Attempts         int            `json:"attempts"`
	DPEpsilon        *float64       `json:"dp_epsilon,omitempty"`
	OutputKey        string         `json:"output_key,omitempty"`
	OutputBytes      int64          `json:"output_bytes,omitempty"`
	OutputKind       string         `json:"output_kind,omitempty"`
	LogsKey          string         `json:"logs_key,omitempty"`
	Error            string         `json:"error,omitempty"`
	CreatedAt        string         `json:"created_at,omitempty"`
	StartedAt        string         `json:"started_at,omitempty"`
	FinishedAt       string         `json:"finished_at,omitempty"`

	// idemKey is the idempotency key for submit dedupe. Unexported: it is a
	// submit-time concern, not part of the wire DTO; the service sets it.
	idemKey string
}

// WithIdempotencyKey returns a copy of the job carrying an idempotency key, so
// repeat submits under one entitlement collapse to a single job (design §4).
func (j Job) WithIdempotencyKey(key string) Job { j.idemKey = key; return j }

// Job lifecycle statuses (design §3). The state machine:
//
//	created → queued → running → output_pending ─┬→ released
//	                                              └→ output_reviewing → released / rejected
//	          running → queued  (crash retry, attempts < max)
//	          running → failed  (attempts exhausted / non-retryable)
//	created → canceled (buyer cancels before run)
const (
	JobCreated         = "created"
	JobQueued          = "queued"
	JobRunning         = "running"
	JobOutputPending   = "output_pending"
	JobOutputReviewing = "output_reviewing"
	JobReleased        = "released"
	JobFailed          = "failed"
	JobRejected        = "rejected"
	JobCanceled        = "canceled"
)

// JobTerminal reports whether a status is final (no further transitions).
func JobTerminal(status string) bool {
	switch status {
	case JobReleased, JobFailed, JobRejected, JobCanceled:
		return true
	}
	return false
}

// --- errors (sentinels; handler maps these to the 7xxx httpx code band) ---

var (
	ErrValidation       = errors.New("validation failed")
	ErrNotFound         = errors.New("resource not found")
	ErrForbidden        = errors.New("not permitted")
	ErrNotVerified      = errors.New("buyer must complete real-name verification")
	ErrOfferDisabled    = errors.New("sandbox compute is not enabled for this dataset")
	ErrAlgoNotAllowed   = errors.New("algorithm is not approved/allowed for this dataset")
	ErrCustomNotAllowed = errors.New("custom algorithms are not allowed on this offer")
	ErrModelNeedsTrust  = errors.New("model output on an L1 offer requires a trusted (audited) algorithm")
	ErrQuotaExhausted   = errors.New("compute entitlement has no remaining job quota")
	ErrEntitlementState = errors.New("entitlement is not active")
	ErrDPBudgetExceeded = errors.New("differential-privacy budget exhausted for this dataset")
	ErrBadTransition    = errors.New("illegal job status transition")
	ErrSelfPurchase     = errors.New("cannot buy compute on your own dataset")
	ErrDuplicateJob     = errors.New("a job with this idempotency key already exists")
	ErrDuplicateEnt     = errors.New("an entitlement for this order already exists")
	ErrPurchasePending  = errors.New("a compute order for this dataset is already in progress")
)
