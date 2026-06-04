package compute

import (
	"context"
	"encoding/json"
	"errors"
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

// OrderCreator creates a compute order through the order module (injected by the
// server so compute doesn't import order). The buyer then pays it through the
// existing payment flow; payment grants the entitlement via GrantForOrder.
type OrderCreator interface {
	CreateComputeOrder(ctx context.Context, buyerID, sellerID, datasetID string, amountCents int64) (orderID string, err error)
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
	orders   OrderCreator // optional; set via SetOrderCreator (real purchase path)

	// Execution engine (optional; set via WithWorker). When runner is nil,
	// SubmitJob leaves the job queued for an out-of-process runner to claim.
	runner       Runner
	store        storage.Storage
	data         DataKeyResolver
	attester     Attester        // optional; verifies stored L2 attestation reports (design P3)
	aggregator   Aggregator      // federated aggregation (P4-a); defaults to FedAvgAggregator
	orchestrator MPCOrchestrator // PSI/MPC orchestration (Direction D); defaults to mockMPC
	runnerID     string
	leaseSecs    int
	maxAttempts  int
	qCh          chan string // queued job ids
	wg           sync.WaitGroup
	stopSweep    chan struct{}
}

// SetOrderCreator wires the real purchase path (compute order via order+payment)
// after construction.
func (s *Service) SetOrderCreator(o OrderCreator) { s.orders = o }

// WithAttester wires an attestation verifier so GetAttestation can re-verify L2
// reports server-side (design P3).
func WithAttester(a Attester) Option { return func(s *Service) { s.attester = a } }

// Option configures optional Service dependencies.
type Option func(*Service)

// NewService builds the compute service.
func NewService(repo Repository, identity IdentityChecker, datasets DatasetReader, rec audit.Recorder, opts ...Option) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	s := &Service{repo: repo, identity: identity, datasets: datasets, audit: rec,
		leaseSecs: DefaultLeaseSecs, maxAttempts: DefaultMaxAttempts,
		aggregator: FedAvgAggregator{}, orchestrator: NewMockMPC()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// WithAggregator overrides the federated aggregation strategy (default FedAvg).
func WithAggregator(a Aggregator) Option { return func(s *Service) { s.aggregator = a } }

// WithOrchestrator overrides the PSI/MPC orchestrator (default in-process mockMPC).
func WithOrchestrator(o MPCOrchestrator) Option { return func(s *Service) { s.orchestrator = o } }

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
	AllowFederated bool // P4-a: opt this dataset into federated use
	AllowPSI       bool // Direction D: opt this dataset into PSI (distinct consent from federated)
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
		AllowFederated: in.AllowFederated, AllowPSI: in.AllowPSI,
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

// PurchaseViaOrder starts a REAL compute purchase: it creates a compute order
// (priced from the offer) through the order module and returns the order id. The
// buyer pays it via the existing payment flow; on payment the entitlement is
// granted by GrantForOrder. (The dev-only direct grant bypasses payment.)
func (s *Service) PurchaseViaOrder(ctx context.Context, buyerID, datasetID string) (string, error) {
	offer, err := s.repo.GetOffer(ctx, datasetID)
	if err != nil {
		return "", err
	}
	if !offer.Enabled {
		return "", ErrOfferDisabled
	}
	ds, err := s.datasets.ForCompute(ctx, datasetID)
	if err != nil {
		return "", err
	}
	if ds.SellerID == buyerID {
		return "", ErrSelfPurchase
	}
	if s.orders == nil {
		return "", fmt.Errorf("%w: order path not configured", ErrValidation)
	}
	orderID, err := s.orders.CreateComputeOrder(ctx, buyerID, ds.SellerID, datasetID, offer.PriceCents)
	if err != nil {
		return "", err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.purchase.order",
		ResourceType: "dataset", ResourceID: datasetID, Detail: map[string]any{"order_id": orderID, "price_cents": offer.PriceCents}})
	return orderID, nil
}

// GrantForOrder grants the compute entitlement for a PAID compute order. Called
// by the order module on payment. Idempotent (one entitlement per order — a
// retried webhook is a no-op).
func (s *Service) GrantForOrder(ctx context.Context, orderID, datasetID, buyerID string) error {
	_, err := s.repo.CreateEntitlement(ctx, Entitlement{
		DatasetID: datasetID, BuyerID: buyerID, OrderID: orderID, JobsQuota: 1,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateEnt) {
			return nil // already granted (idempotent)
		}
		return err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.entitlement.grant_paid",
		ResourceType: "compute_order", ResourceID: orderID, Detail: map[string]any{"dataset_id": datasetID}})
	return nil
}

// RevokeEntitlementsForOrder revokes compute credits when an order is refunded
// (H2 linkage). Returns the count revoked.
func (s *Service) RevokeEntitlementsForOrder(ctx context.Context, orderID string) (int, error) {
	return s.repo.RevokeByOrder(ctx, orderID)
}

// ListEntitlements returns a buyer's compute entitlements (so the UI doesn't
// have to remember them client-side).
func (s *Service) ListEntitlements(ctx context.Context, buyerID string, limit, offset int) ([]Entitlement, error) {
	return s.repo.ListEntitlementsByBuyer(ctx, buyerID, clampLimit(limit), max0(offset))
}

// --- ops: output review (for offers with review_output) ---

// OpsReleaseOutput releases a job parked in output_reviewing to the buyer.
func (s *Service) OpsReleaseOutput(ctx context.Context, opsID, jobID string) (Job, error) {
	j, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	if j.Status != JobOutputReviewing {
		return Job{}, ErrBadTransition
	}
	out, err := s.repo.Release(ctx, jobID, j.OutputKey, j.OutputKind, j.OutputBytes, j.LogsKey)
	if err != nil {
		return Job{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: opsID, Action: "compute.job.ops_release",
		ResourceType: "compute_job", ResourceID: jobID})
	return out, nil
}

// OpsRejectOutput rejects a job parked in output_reviewing (output withheld) and
// refunds the buyer's credit (§21).
func (s *Service) OpsRejectOutput(ctx context.Context, opsID, jobID, reason string) (Job, error) {
	j, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	if j.Status != JobOutputReviewing {
		return Job{}, ErrBadTransition
	}
	out, err := s.repo.Reject(ctx, jobID, orReason(reason, "ops_review_rejected"))
	if err != nil {
		return Job{}, err
	}
	if err := s.repo.RefundQuota(ctx, j.EntitlementID); err != nil {
		slog.Error("compute: quota refund on ops reject failed", "job_id", jobID, "err", err)
	}
	s.audit.Record(ctx, audit.Entry{ActorID: opsID, Action: "compute.job.ops_reject",
		ResourceType: "compute_job", ResourceID: jobID, Detail: map[string]any{"reason": reason}})
	return out, nil
}

func orReason(s, def string) string {
	if s == "" {
		return def
	}
	return s
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
	return s.submitJobTagged(ctx, buyerID, in, "")
}

// submitJobTagged is the shared submit path. federatedID is "" for a normal
// single-dataset job, or the parent federated job's id for a federated sub-job
// (whose output is internal-only and feeds aggregation, never released to the buyer).
func (s *Service) submitJobTagged(ctx context.Context, buyerID string, in SubmitInput, federatedID string) (Job, error) {
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
		Status: JobQueued, DPEpsilon: jobEps, FederatedJobID: federatedID,
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
	if j.FederatedJobID != "" {
		return Job{}, ErrForbidden // federated sub-jobs are internal; use the federated job APIs
	}
	return j, nil
}

// ListJobs returns the buyer's jobs.
func (s *Service) ListJobs(ctx context.Context, buyerID string, limit, offset int) ([]Job, error) {
	return s.repo.ListJobsByBuyer(ctx, buyerID, clampLimit(limit), max0(offset))
}

// GetAttestation returns a job's L2 remote-attestation report, re-verified
// server-side when an Attester is wired (design P3). Viewable by the job's buyer
// or the dataset's seller. ErrNotFound if the job has no attestation (e.g. L1).
func (s *Service) GetAttestation(ctx context.Context, userID, jobID string) (Attestation, error) {
	j, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return Attestation{}, err
	}
	if j.BuyerID != userID {
		ds, derr := s.datasets.ForCompute(ctx, j.DatasetID)
		if derr != nil {
			return Attestation{}, derr
		}
		if ds.SellerID != userID {
			return Attestation{}, ErrForbidden
		}
	}
	if len(j.Attestation) == 0 {
		return Attestation{}, ErrNotFound
	}
	report, _ := json.Marshal(j.Attestation)
	if s.attester != nil {
		return s.attester.Verify(ctx, report)
	}
	var a Attestation
	if err := json.Unmarshal(report, &a); err != nil {
		return Attestation{}, err
	}
	return a, nil
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
	if j.FederatedJobID != "" {
		return Job{}, ErrForbidden // federated sub-jobs are coordinated internally, not cancelable directly
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
