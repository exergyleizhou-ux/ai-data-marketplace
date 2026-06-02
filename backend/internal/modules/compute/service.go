package compute

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// IdentityChecker reports a user's KYC status (implemented by auth).
type IdentityChecker interface {
	KYCStatus(ctx context.Context, userID string) (string, error)
}

// DatasetInfo is the subset of dataset state the compute module needs.
type DatasetInfo struct {
	SellerID  string
	VersionID string // current published version
	Published bool
}

// DatasetReader exposes compute-relevant dataset info (implemented by dataset,
// bridged by the server so neither package imports the other).
type DatasetReader interface {
	ForCompute(ctx context.Context, datasetID string) (DatasetInfo, error)
}

// DataKeyResolver returns a dataset's current object-storage key so the runner
// can stage the data read-only (implemented by dataset).
type DataKeyResolver interface {
	CurrentObjectKey(ctx context.Context, datasetID string) (string, error)
}

const kycVerified = "verified"

// Lease / retry tuning for the worker.
const (
	DefaultLeaseSecs   = 120
	DefaultMaxAttempts = 3
)

// Service holds compute-to-data business logic and drives the job state machine.
// Cross-module dependencies are interfaces injected by the server.
type Service struct {
	repo     Repository
	identity IdentityChecker
	datasets DatasetReader
	audit    audit.Recorder

	// Execution engine (optional; set via WithWorker). When runner is nil,
	// SubmitJob leaves the job queued for an out-of-process runner to claim.
	runner      Runner
	store       storage.Storage
	data        DataKeyResolver
	runnerID    string
	leaseSecs   int
	maxAttempts int
	qCh         chan string // queued job ids
	wg          sync.WaitGroup
	stopSweep   chan struct{}
}

// Option configures optional Service dependencies.
type Option func(*Service)

// NewService builds the compute service.
func NewService(repo Repository, identity IdentityChecker, datasets DatasetReader, rec audit.Recorder, opts ...Option) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	s := &Service{repo: repo, identity: identity, datasets: datasets, audit: rec,
		leaseSecs: DefaultLeaseSecs, maxAttempts: DefaultMaxAttempts}
	for _, o := range opts {
		o(s)
	}
	return s
}

// --- seller: offer configuration ---

// OfferInput is a seller's sandbox-sale configuration for a dataset.
type OfferInput struct {
	Enabled        bool
	AllowCustom    bool
	AllowedAlgoIDs []string
	PriceCents     int64
	MaxRuntimeSecs int
	MaxOutputBytes int64
	MaxOutputFiles int
	DPEpsilon      *float64
	DPEpsilonTotal *float64
	ReturnLogs     bool
	ReviewOutput   bool
	TrustLevel     string
}

// ConfigureOffer lets the dataset's seller enable/configure sandbox sale.
func (s *Service) ConfigureOffer(ctx context.Context, sellerID, datasetID string, in OfferInput) (Offer, error) {
	ds, err := s.datasets.ForCompute(ctx, datasetID)
	if err != nil {
		return Offer{}, err
	}
	if ds.SellerID != sellerID {
		return Offer{}, ErrForbidden
	}
	switch in.TrustLevel {
	case "", TrustL1:
		in.TrustLevel = TrustL1
	case TrustL2, TrustL3:
		// allowed; runner support lands in later phases
	default:
		return Offer{}, fmt.Errorf("%w: invalid trust_level", ErrValidation)
	}
	if in.PriceCents < 0 {
		return Offer{}, fmt.Errorf("%w: price_cents must be >= 0", ErrValidation)
	}
	if in.MaxRuntimeSecs <= 0 {
		in.MaxRuntimeSecs = 1800
	}
	if in.MaxOutputBytes <= 0 {
		in.MaxOutputBytes = 10 << 20
	}
	if in.MaxOutputFiles <= 0 {
		in.MaxOutputFiles = 16
	}
	o, err := s.repo.UpsertOffer(ctx, Offer{
		DatasetID: datasetID, Enabled: in.Enabled, AllowCustom: in.AllowCustom,
		AllowedAlgoIDs: in.AllowedAlgoIDs, PriceCents: in.PriceCents,
		MaxRuntimeSecs: in.MaxRuntimeSecs, MaxOutputBytes: in.MaxOutputBytes, MaxOutputFiles: in.MaxOutputFiles,
		DPEpsilon: in.DPEpsilon, DPEpsilonTotal: in.DPEpsilonTotal,
		ReturnLogs: in.ReturnLogs, ReviewOutput: in.ReviewOutput, TrustLevel: in.TrustLevel,
	})
	if err != nil {
		return Offer{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: sellerID, Action: "compute.offer.configure",
		ResourceType: "dataset", ResourceID: datasetID,
		Detail: map[string]any{"enabled": in.Enabled, "trust_level": in.TrustLevel, "price_cents": in.PriceCents}})
	return o, nil
}

// GetOffer returns a dataset's offer (public read for the buyer's product page).
func (s *Service) GetOffer(ctx context.Context, datasetID string) (Offer, error) {
	return s.repo.GetOffer(ctx, datasetID)
}

// ListAlgorithms returns the approved algorithms a buyer may run on a dataset:
// the platform-approved set, intersected with the offer's allowlist when set.
func (s *Service) ListAlgorithms(ctx context.Context, datasetID string) ([]Algorithm, error) {
	offer, err := s.repo.GetOffer(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	if !offer.Enabled {
		return nil, ErrOfferDisabled
	}
	approved, err := s.repo.ListApprovedAlgorithms(ctx)
	if err != nil {
		return nil, err
	}
	allow := map[string]bool{}
	for _, id := range offer.AllowedAlgoIDs {
		allow[id] = true
	}
	var out []Algorithm
	for _, a := range approved {
		if len(allow) > 0 && !allow[a.ID] {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// --- algorithm registry (ops) ---

// RegisterAlgorithm records a new algorithm (ops/platform). Trusted algorithms
// must pin an image digest and cite a source — the audited code, not the
// sandbox, is the boundary that makes model output safe (design §7.3).
func (s *Service) RegisterAlgorithm(ctx context.Context, a Algorithm) (Algorithm, error) {
	if a.Name == "" || a.Runtime == "" || a.Image == "" || a.OutputKind == "" {
		return Algorithm{}, fmt.Errorf("%w: name, runtime, image, output_kind required", ErrValidation)
	}
	switch a.OutputKind {
	case OutputModel, OutputMetrics, OutputTable, OutputAggregate:
	default:
		return Algorithm{}, fmt.Errorf("%w: invalid output_kind", ErrValidation)
	}
	if a.Trusted && (a.ImageDigest == "" || a.SourceRef == "") {
		return Algorithm{}, fmt.Errorf("%w: trusted algorithm must pin image_digest and cite source_ref", ErrValidation)
	}
	return s.repo.RegisterAlgorithm(ctx, a)
}

// ReviewAlgorithm is the ops approval decision (approved/rejected/disabled,
// trusted flag). Approving as trusted requires a pinned digest + source.
func (s *Service) ReviewAlgorithm(ctx context.Context, opsID, id, status string, trusted bool) (Algorithm, error) {
	switch status {
	case AlgoApproved, AlgoRejected, AlgoDisabled, AlgoPending:
	default:
		return Algorithm{}, fmt.Errorf("%w: invalid status", ErrValidation)
	}
	a, err := s.repo.GetAlgorithm(ctx, id)
	if err != nil {
		return Algorithm{}, err
	}
	if trusted && (a.ImageDigest == "" || a.SourceRef == "") {
		return Algorithm{}, fmt.Errorf("%w: cannot trust an algorithm without a pinned image_digest and source_ref", ErrValidation)
	}
	out, err := s.repo.ReviewAlgorithm(ctx, id, status, trusted)
	if err != nil {
		return Algorithm{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: opsID, Action: "compute.algorithm.review",
		ResourceType: "algorithm", ResourceID: id, Detail: map[string]any{"status": status, "trusted": trusted}})
	return out, nil
}

// AdminListAlgorithms lists algorithms by review status (ops console).
func (s *Service) AdminListAlgorithms(ctx context.Context, status string) ([]Algorithm, error) {
	if status == "" {
		status = AlgoPending
	}
	return s.repo.ListAlgorithmsByStatus(ctx, status, 100)
}

// AdminListJobs lists jobs by status (ops console; e.g. output_reviewing queue).
func (s *Service) AdminListJobs(ctx context.Context, status string, limit int) ([]Job, error) {
	return s.repo.ListJobsByStatus(ctx, status, clampLimit(limit))
}

// --- entitlements ---

// GrantEntitlement creates compute credits for a buyer after payment succeeds
// (called by the purchase flow / payment wiring). orderID may be "".
func (s *Service) GrantEntitlement(ctx context.Context, datasetID, buyerID, orderID string, quota int) (Entitlement, error) {
	if quota < 1 {
		quota = 1
	}
	e, err := s.repo.CreateEntitlement(ctx, Entitlement{
		DatasetID: datasetID, BuyerID: buyerID, OrderID: orderID, JobsQuota: quota,
	})
	if err != nil {
		return Entitlement{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.entitlement.grant",
		ResourceType: "dataset", ResourceID: datasetID, Detail: map[string]any{"order_id": orderID, "quota": quota}})
	return e, nil
}

// RevokeEntitlementsForOrder revokes compute credits when an order is refunded
// (H2 linkage). Returns the count revoked.
func (s *Service) RevokeEntitlementsForOrder(ctx context.Context, orderID string) (int, error) {
	return s.repo.RevokeByOrder(ctx, orderID)
}

// --- buyer: job submission ---

// SubmitInput is a buyer's request to run an algorithm on a dataset.
type SubmitInput struct {
	DatasetID      string
	EntitlementID  string
	AlgorithmID    string
	Params         map[string]any
	IdempotencyKey string
}

// SubmitJob validates the request against every L1 invariant, atomically
// consumes one entitlement credit, and creates a queued job. The order matters:
// idempotency pre-check → algorithm/offer validation → DP budget → atomic quota
// spend → create. On a duplicate-key race the spent quota is refunded.
func (s *Service) SubmitJob(ctx context.Context, buyerID string, in SubmitInput) (Job, error) {
	offer, err := s.repo.GetOffer(ctx, in.DatasetID)
	if err != nil {
		return Job{}, err
	}
	if !offer.Enabled {
		return Job{}, ErrOfferDisabled
	}
	ent, err := s.repo.GetEntitlement(ctx, in.EntitlementID)
	if err != nil {
		return Job{}, err
	}
	if ent.BuyerID != buyerID {
		return Job{}, ErrForbidden
	}
	if ent.DatasetID != in.DatasetID {
		return Job{}, fmt.Errorf("%w: entitlement is for a different dataset", ErrValidation)
	}
	// Entitlement state/quota is enforced atomically by SpendQuota below (the
	// single source of truth), which returns ErrQuotaExhausted vs
	// ErrEntitlementState precisely. Reject the clearly-terminal states early
	// for a faster, clearer error.
	if ent.Status == EntRevoked || ent.Status == EntExpired {
		return Job{}, ErrEntitlementState
	}

	status, err := s.identity.KYCStatus(ctx, buyerID)
	if err != nil {
		return Job{}, err
	}
	if status != kycVerified {
		return Job{}, ErrNotVerified
	}

	ds, err := s.datasets.ForCompute(ctx, in.DatasetID)
	if err != nil {
		return Job{}, err
	}
	if ds.SellerID == buyerID {
		return Job{}, ErrSelfPurchase
	}

	algo, err := s.resolveAlgorithm(ctx, offer, in.AlgorithmID)
	if err != nil {
		return Job{}, err
	}

	// Idempotency pre-check: a repeat submit returns the existing job WITHOUT
	// spending another credit.
	if in.IdempotencyKey != "" {
		if existing, gerr := s.repo.GetJobByIdempotency(ctx, in.EntitlementID, in.IdempotencyKey); gerr == nil {
			return existing, nil
		}
	}

	// DP budget pre-check for aggregate/metric outputs (design §8).
	var jobEps *float64
	if (algo.OutputKind == OutputAggregate || algo.OutputKind == OutputMetrics) && offer.DPEpsilon != nil {
		eps := *offer.DPEpsilon
		jobEps = &eps
		if offer.DPEpsilonTotal != nil {
			spent, serr := s.repo.SumDP(ctx, in.DatasetID, buyerID)
			if serr != nil {
				return Job{}, serr
			}
			if spent+eps > *offer.DPEpsilonTotal {
				return Job{}, ErrDPBudgetExceeded
			}
		}
	}

	// Atomic quota spend (the gate against over-submission).
	if _, err := s.repo.SpendQuota(ctx, in.EntitlementID); err != nil {
		return Job{}, err
	}

	job := Job{
		DatasetID: in.DatasetID, VersionID: ds.VersionID, BuyerID: buyerID, EntitlementID: in.EntitlementID,
		AlgorithmID: algo.ID, AlgorithmVersion: algo.Version, Params: in.Params,
		Status: JobQueued, DPEpsilon: jobEps,
	}.WithIdempotencyKey(in.IdempotencyKey)

	out, err := s.repo.CreateJob(ctx, job)
	if err != nil {
		// Compensate the spent credit; on an idempotency race return the winner.
		if rerr := s.repo.RefundQuota(ctx, in.EntitlementID); rerr != nil {
			slog.Error("quota refund after failed job create failed", "entitlement_id", in.EntitlementID, "err", rerr)
		}
		if err == ErrDuplicateJob && in.IdempotencyKey != "" {
			if existing, gerr := s.repo.GetJobByIdempotency(ctx, in.EntitlementID, in.IdempotencyKey); gerr == nil {
				return existing, nil
			}
		}
		return Job{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.job.submit",
		ResourceType: "compute_job", ResourceID: out.ID,
		Detail: map[string]any{"dataset_id": in.DatasetID, "algorithm_id": algo.ID, "output_kind": algo.OutputKind}})
	s.enqueue(out.ID)
	return out, nil
}

// resolveAlgorithm validates the requested algorithm against the offer and the
// L1 security invariants (design §2 / §7.3):
//   - the algorithm must be approved and allowed by the offer
//   - custom algorithms require offer.allow_custom (and are not supported in P1)
//   - on an L1 offer, MODEL output requires a trusted (audited) algorithm
func (s *Service) resolveAlgorithm(ctx context.Context, offer Offer, algorithmID string) (Algorithm, error) {
	if algorithmID == "" {
		// No whitelisted algorithm chosen ⇒ this would be a custom algorithm.
		if !offer.AllowCustom {
			return Algorithm{}, ErrCustomNotAllowed
		}
		// P1 ships whitelist-only; custom execution is a later phase.
		return Algorithm{}, fmt.Errorf("%w: custom algorithms are not yet supported", ErrCustomNotAllowed)
	}
	algo, err := s.repo.GetAlgorithm(ctx, algorithmID)
	if err != nil {
		return Algorithm{}, err
	}
	if algo.Status != AlgoApproved {
		return Algorithm{}, ErrAlgoNotAllowed
	}
	if len(offer.AllowedAlgoIDs) > 0 {
		ok := false
		for _, id := range offer.AllowedAlgoIDs {
			if id == algorithmID {
				ok = true
				break
			}
		}
		if !ok {
			return Algorithm{}, ErrAlgoNotAllowed
		}
	}
	// HARD INVARIANT: L1 model output ⇒ trusted (audited) algorithm only.
	if offer.TrustLevel == TrustL1 && algo.OutputKind == OutputModel && !algo.Trusted {
		return Algorithm{}, ErrModelNeedsTrust
	}
	return algo, nil
}

// --- buyer/ops: job reads ---

// GetJob returns a job; only its buyer may view it.
func (s *Service) GetJob(ctx context.Context, userID, id string) (Job, error) {
	j, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if j.BuyerID != userID {
		return Job{}, ErrForbidden
	}
	return j, nil
}

// ListJobs returns the buyer's jobs.
func (s *Service) ListJobs(ctx context.Context, buyerID string, limit, offset int) ([]Job, error) {
	return s.repo.ListJobsByBuyer(ctx, buyerID, clampLimit(limit), max0(offset))
}

// CancelJob lets the buyer cancel a job that has not started running, refunding
// the credit.
func (s *Service) CancelJob(ctx context.Context, buyerID, id string) (Job, error) {
	j, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if j.BuyerID != buyerID {
		return Job{}, ErrForbidden
	}
	out, err := s.repo.Transition(ctx, id, JobQueued, JobCanceled)
	if err != nil {
		// Also allow canceling a freshly-created (not yet queued) job.
		if out, err = s.repo.Transition(ctx, id, JobCreated, JobCanceled); err != nil {
			return Job{}, err
		}
	}
	if rerr := s.repo.RefundQuota(ctx, j.EntitlementID); rerr != nil {
		slog.Error("quota refund on cancel failed", "entitlement_id", j.EntitlementID, "err", rerr)
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.job.cancel",
		ResourceType: "compute_job", ResourceID: id})
	return out, nil
}

func clampLimit(l int) int {
	if l <= 0 || l > 100 {
		return 20
	}
	return l
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
