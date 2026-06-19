package compute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/metrics"
)

// fedMaxOutputBytes is a defensive cap on the aggregated joint model size. The
// partials already passed each dataset's per-offer output gate; FedAvg produces
// a same-dimension model, so this only guards against pathological inputs.
const fedMaxOutputBytes = 64 << 20

// FederatedSubmitInput is a buyer's request to run one federated job across N datasets.
type FederatedSubmitInput struct {
	AlgorithmID     string
	DatasetIDs      []string
	Params          map[string]any
	DPEpsilon       *float64
	MinParticipants int // 0 ⇒ all datasets (every party required); else tolerate dropouts down to this many
}

// SubmitFederatedJob validates each participating dataset (offer enabled +
// allow_federated + the buyer holds an active entitlement), creates the federated
// job, then fans out one sandbox sub-job per dataset (each spending that dataset's
// entitlement). Each seller's data only ever runs inside its own sandbox; only
// model params are later aggregated (design P4 §2.1).
func (s *Service) SubmitFederatedJob(ctx context.Context, buyerID string, in FederatedSubmitInput) (FederatedJob, error) {
	if len(in.DatasetIDs) < 2 {
		return FederatedJob{}, ErrFederatedParties
	}
	// Resolve the minimum participants: 0 ⇒ require every party; else 2..N.
	minP := in.MinParticipants
	if minP <= 0 {
		minP = len(in.DatasetIDs)
	}
	if minP < 2 || minP > len(in.DatasetIDs) {
		return FederatedJob{}, fmt.Errorf("%w: min_participants must be between 2 and the number of datasets", ErrValidation)
	}
	// The job's aggregation mode follows the algorithm's runtime: a psi-extract
	// algorithm makes this a private set intersection (Direction D), otherwise it
	// is FedAvg federated learning. An unknown/unfetchable algorithm defaults to
	// federated — the per-sub-job validation still rejects an invalid algorithm.
	mode := ModeFederated
	if algo, aerr := s.repo.GetAlgorithm(ctx, in.AlgorithmID); aerr == nil && algo.Runtime == RuntimePSIExtract {
		mode = ModePSI
	}

	// Pre-flight every dataset BEFORE creating anything, so we never fan out a
	// partially-valid job. Each mode needs its own seller consent: federated and
	// PSI are distinct privacy exposures (co-train a model vs. reveal set overlap),
	// so allow_federated does NOT imply allow_psi.
	entByDataset := make(map[string]string, len(in.DatasetIDs))
	for _, ds := range in.DatasetIDs {
		offer, err := s.repo.GetOffer(ctx, ds)
		if err != nil {
			return FederatedJob{}, err
		}
		consented := offer.AllowFederated
		useLabel := "federated"
		if mode == ModePSI {
			consented = offer.AllowPSI
			useLabel = "PSI"
		}
		if !offer.Enabled || !consented {
			return FederatedJob{}, fmt.Errorf("%w: dataset %s does not allow %s use", ErrOfferDisabled, ds, useLabel)
		}
		ent, err := s.activeEntitlementFor(ctx, buyerID, ds)
		if err != nil {
			return FederatedJob{}, err
		}
		entByDataset[ds] = ent.ID
	}

	fed, err := s.repo.CreateFederatedJob(ctx, FederatedJob{
		BuyerID: buyerID, AlgorithmID: in.AlgorithmID, DatasetIDs: in.DatasetIDs,
		Mode: mode, MinParticipants: minP, Params: in.Params, DPEpsilon: in.DPEpsilon,
	})
	if err != nil {
		return FederatedJob{}, err
	}

	// Fan out: one sandbox sub-job per dataset, tagged with the federated id.
	created := make([]Job, 0, len(in.DatasetIDs))
	for _, ds := range in.DatasetIDs {
		sub, err := s.submitJobTagged(ctx, buyerID, SubmitInput{
			DatasetID: ds, EntitlementID: entByDataset[ds], AlgorithmID: in.AlgorithmID, Params: in.Params,
		}, fed.ID)
		if err != nil {
			// Roll back: fail the federated job and refund every sub-job created so far.
			_, _ = s.repo.FailFederated(ctx, fed.ID, "fanout_failed")
			s.refundFederated(ctx, created)
			return FederatedJob{}, err
		}
		created = append(created, sub)
	}

	fed, err = s.repo.TransitionFederated(ctx, fed.ID, FedCreated, FedFanout)
	if err != nil {
		return FederatedJob{}, err
	}
	metrics.RecordFederatedParticipants("submitted", len(in.DatasetIDs))
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.federated.submit",
		ResourceType: "compute_federated_job", ResourceID: fed.ID,
		Detail: map[string]any{"datasets": len(in.DatasetIDs), "algorithm_id": in.AlgorithmID}})
	// Startup-race guard: sub-jobs run very fast (esp. MockRunner) and may all
	// finish BEFORE this fanout transition — their tryAdvance callbacks no-op while
	// status is still 'created'. Without this explicit kick nothing would advance
	// the job and it would hang in 'fanout'. Safe + idempotent (state-transition gated).
	s.tryAdvanceFederated(ctx, fed.ID)
	return fed, nil
}

// ListFederatedJobs returns the buyer's federated jobs, newest first.
func (s *Service) ListFederatedJobs(ctx context.Context, buyerID string, limit, offset int) ([]FederatedJob, error) {
	return s.repo.ListFederatedJobsByBuyer(ctx, buyerID, limit, offset)
}

// activeEntitlementFor finds the buyer's first active entitlement with remaining
// quota for a dataset.
func (s *Service) activeEntitlementFor(ctx context.Context, buyerID, datasetID string) (Entitlement, error) {
	ents, err := s.repo.ListEntitlementsByBuyer(ctx, buyerID, 100, 0)
	if err != nil {
		return Entitlement{}, err
	}
	for _, e := range ents {
		if e.DatasetID == datasetID && e.Status == EntActive && e.JobsUsed < e.JobsQuota {
			return e, nil
		}
	}
	return Entitlement{}, fmt.Errorf("%w: dataset %s", ErrQuotaExhausted, datasetID)
}

// GetFederatedJob returns the federated job and its sub-jobs for the owning buyer.
func (s *Service) GetFederatedJob(ctx context.Context, userID, id string) (FederatedJob, []Job, error) {
	fed, err := s.repo.GetFederatedJob(ctx, id)
	if err != nil {
		return FederatedJob{}, nil, err
	}
	if fed.BuyerID != userID {
		return FederatedJob{}, nil, ErrForbidden
	}
	subs, err := s.repo.ListSubJobs(ctx, id)
	if err != nil {
		return FederatedJob{}, nil, err
	}
	return fed, subs, nil
}

// OpenFederatedOutput streams the released joint model to the owning buyer.
func (s *Service) OpenFederatedOutput(ctx context.Context, userID, id string) (io.ReadCloser, int64, FederatedJob, error) {
	fed, err := s.repo.GetFederatedJob(ctx, id)
	if err != nil {
		return nil, 0, FederatedJob{}, err
	}
	if fed.BuyerID != userID {
		return nil, 0, FederatedJob{}, ErrForbidden
	}
	if fed.Status != FedReleased || fed.OutputKey == "" {
		return nil, 0, FederatedJob{}, ErrOutputNotReady
	}
	if s.store == nil {
		return nil, 0, FederatedJob{}, ErrNotFound
	}
	rc, n, err := s.store.Open(ctx, fed.OutputKey)
	if err != nil {
		return nil, 0, FederatedJob{}, err
	}
	return rc, n, fed, nil
}

// GetFederatedCertificate returns a provenance & integrity certificate for a
// buyer's released joint result (federated model or PSI intersection): it hashes
// the joint output and binds it to the audited algorithm and the participating
// datasets (存证, extends the compute-result certificate to L3 results).
func (s *Service) GetFederatedCertificate(ctx context.Context, userID, id string) (map[string]any, error) {
	fed, err := s.repo.GetFederatedJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if fed.BuyerID != userID {
		return nil, ErrForbidden
	}
	if fed.Status != FedReleased || fed.OutputKey == "" {
		return BuildFederatedCertificate(fed, Algorithm{}, ""), nil // pending
	}
	if s.store == nil {
		return nil, ErrNotFound
	}
	rc, _, err := s.store.Open(ctx, fed.OutputKey)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		rc.Close()
		return nil, err
	}
	rc.Close()
	outputSHA := hex.EncodeToString(h.Sum(nil))

	var algo Algorithm
	if fed.AlgorithmID != "" {
		algo, _ = s.repo.GetAlgorithm(ctx, fed.AlgorithmID)
	}
	return BuildFederatedCertificate(fed, algo, outputSHA), nil
}

// tryAdvanceFederated is called after each sub-job reaches a terminal state. It
// waits until EVERY sub-job has settled, then decides once (idempotent via the
// state transition): if at least min_participants released, it aggregates the
// survivors (refunding the dropouts, who aren't billed); otherwise it fails the
// whole job and refunds everyone (the buyer got no usable output). Tolerating
// dropouts down to min_participants is the federated fault-tolerance contract
// (design P4 §4).
func (s *Service) tryAdvanceFederated(ctx context.Context, fedID string) {
	fed, err := s.repo.GetFederatedJob(ctx, fedID)
	if err != nil || fed.Status != FedFanout {
		return
	}
	subs, err := s.repo.ListSubJobs(ctx, fedID)
	if err != nil {
		return
	}
	var released, dropouts []Job
	pending := 0
	for _, j := range subs {
		switch j.Status {
		case JobReleased:
			released = append(released, j)
		case JobFailed, JobRejected, JobCanceled:
			dropouts = append(dropouts, j)
		default:
			pending++
		}
	}
	if pending > 0 {
		return // wait until every sub-job has settled before deciding
	}
	// All settled. Proceed if enough parties succeeded; otherwise fail the job.
	if len(released) >= fed.MinParticipants && len(released) >= 1 {
		if _, err := s.repo.TransitionFederated(ctx, fedID, FedFanout, FedAggregating); err != nil {
			return // another goroutine won the race
		}
		metrics.RecordFederatedParticipants("survived", len(released))
		metrics.RecordFederatedParticipants("dropped", len(dropouts))
		s.refundFederated(ctx, dropouts)
		s.aggregateAndRelease(ctx, fed, released)
		return
	}
	// Too few survivors → fail and refund everyone (released + dropouts) once.
	if _, ferr := s.repo.FailFederated(ctx, fedID, "insufficient_participants"); ferr == nil {
		metrics.RecordFederatedJob("failed")
		metrics.RecordFederatedParticipants("dropped", len(subs))
		s.refundFederated(ctx, subs)
	}
}

// aggregateAndRelease reads each released sub-job's local params, runs the
// aggregator, gates the joint output, and releases it. `subs` is the set of
// released participants (dropouts are excluded and refunded by the caller). On
// any error it fails the federated job and refunds these released participants.
func (s *Service) aggregateAndRelease(ctx context.Context, fed FederatedJob, subs []Job) {
	if fed.Mode == ModePSI {
		s.aggregatePSIAndRelease(ctx, fed, subs)
		return
	}
	aggStart := time.Now()
	partials := make([]Partial, 0, len(subs))
	for _, j := range subs {
		rc, _, err := s.store.Open(ctx, j.OutputKey)
		if err != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "read_partial", subs)
			return
		}
		raw, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "read_partial", subs)
			return
		}
		p, perr := parsePartial(raw)
		if perr != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "bad_partial", subs)
			return
		}
		partials = append(partials, p)
	}
	// When the federated job sets dp_epsilon, release a central-DP (Laplace) noised
	// joint model instead of the raw mean (design §5; honest central DP, not DP-SGD).
	var joint []byte
	var err error
	if fed.DPEpsilon != nil {
		clip := defaultDPClip
		if c, ok := fed.Params["dp_clip"].(float64); ok && c > 0 {
			clip = c
		}
		joint, err = dpFedAvg(partials, *fed.DPEpsilon, clip, laplaceNoise)
	} else {
		joint, err = s.aggregator.Aggregate(partials)
	}
	if err != nil {
		s.failFederatedWithRefund(ctx, fed.ID, "aggregate_error", subs)
		return
	}
	if int64(len(joint)) > fedMaxOutputBytes {
		s.failFederatedWithRefund(ctx, fed.ID, "joint_output_too_large", subs)
		return
	}
	key := "compute/federated/" + fed.ID + "/model.json"
	size, err := uploadOutput(ctx, s.store, key, joint)
	if err != nil {
		s.failFederatedWithRefund(ctx, fed.ID, "store_error", subs)
		return
	}
	if _, err := s.repo.ReleaseFederated(ctx, fed.ID, key, size); err != nil {
		slog.Error("compute: release federated failed", "federated_job_id", fed.ID, "err", err)
		return
	}
	// Make the joint result publicly verifiable at /verify/<cert_id>, like a
	// regular released job (the buyer-facing federated certificate already exposes
	// this id; without this it was absent from the public index).
	s.registerFederatedResultCert(ctx, fed.ID, joint)
	metrics.ObserveFederatedAggregation(time.Since(aggStart).Seconds())
	metrics.RecordFederatedJob("released")
	// Record DP spend per participating dataset (ledger keys on the sub-job id,
	// which is a real compute_jobs row — design §8 budget tracking).
	if fed.DPEpsilon != nil {
		for _, j := range subs {
			if err := s.repo.SpendDP(ctx, j.DatasetID, fed.BuyerID, j.ID, *fed.DPEpsilon, nil); err != nil {
				slog.Error("compute: federated dp ledger write failed", "federated_job_id", fed.ID, "sub_job", j.ID, "err", err)
			}
		}
	}
	s.audit.Record(ctx, audit.Entry{Action: "compute.federated.release", ResourceType: "compute_federated_job",
		ResourceID: fed.ID, Detail: map[string]any{"participants": len(partials), "output_bytes": size, "dp": fed.DPEpsilon != nil}})
}

// aggregatePSIAndRelease reads each released sub-job's party set and computes a
// private set intersection (Direction D), releasing the intersection as the joint
// output. Mirrors aggregateAndRelease's gating/refund contract. 阶段1 uses the
// in-process mockMPC (platform-visible); 阶段2 delegates to Secretflow/SPU.
func (s *Service) aggregatePSIAndRelease(ctx context.Context, fed FederatedJob, subs []Job) {
	aggStart := time.Now()
	parties := make([][]string, 0, len(subs))
	for _, j := range subs {
		rc, _, err := s.store.Open(ctx, j.OutputKey)
		if err != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "read_partial", subs)
			return
		}
		raw, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "read_partial", subs)
			return
		}
		set, perr := parsePSISet(raw)
		if perr != nil {
			s.failFederatedWithRefund(ctx, fed.ID, "bad_partial", subs)
			return
		}
		parties = append(parties, set)
	}
	res, err := s.orchestrator.RunPSI(ctx, parties)
	if err != nil {
		s.failFederatedWithRefund(ctx, fed.ID, "psi_error", subs)
		return
	}
	joint, err := marshalPSIResult(res, len(parties))
	if err != nil {
		s.failFederatedWithRefund(ctx, fed.ID, "psi_error", subs)
		return
	}
	if int64(len(joint)) > fedMaxOutputBytes {
		s.failFederatedWithRefund(ctx, fed.ID, "joint_output_too_large", subs)
		return
	}
	key := "compute/federated/" + fed.ID + "/psi-result.json"
	size, err := uploadOutput(ctx, s.store, key, joint)
	if err != nil {
		s.failFederatedWithRefund(ctx, fed.ID, "store_error", subs)
		return
	}
	if _, err := s.repo.ReleaseFederated(ctx, fed.ID, key, size); err != nil {
		slog.Error("compute: release psi failed", "federated_job_id", fed.ID, "err", err)
		return
	}
	s.registerFederatedResultCert(ctx, fed.ID, joint) // publicly verifiable, like the FedAvg path
	metrics.ObserveFederatedAggregation(time.Since(aggStart).Seconds())
	metrics.RecordFederatedJob("released")
	s.audit.Record(ctx, audit.Entry{Action: "compute.federated.psi.release", ResourceType: "compute_federated_job",
		ResourceID: fed.ID, Detail: map[string]any{"participants": len(parties), "cardinality": res.Cardinality, "output_bytes": size}})
}

// failFederatedWithRefund fails the job (idempotently) and refunds all sub-jobs once.
func (s *Service) failFederatedWithRefund(ctx context.Context, fedID, code string, subs []Job) {
	if _, err := s.repo.FailFederated(ctx, fedID, code); err == nil {
		metrics.RecordFederatedJob("failed")
		s.refundFederated(ctx, subs)
	}
}

// refundFederated refunds each sub-job's entitlement exactly once. Callers gate
// this behind a successful FailFederated transition so it runs at most once.
func (s *Service) refundFederated(ctx context.Context, subs []Job) {
	for _, j := range subs {
		if j.EntitlementID == "" {
			continue
		}
		if err := s.repo.RefundQuota(ctx, j.EntitlementID); err != nil {
			slog.Error("compute: federated refund failed", "entitlement_id", j.EntitlementID, "err", err)
		}
	}
}
