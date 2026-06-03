package compute

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// fedMaxOutputBytes is a defensive cap on the aggregated joint model size. The
// partials already passed each dataset's per-offer output gate; FedAvg produces
// a same-dimension model, so this only guards against pathological inputs.
const fedMaxOutputBytes = 64 << 20

// FederatedSubmitInput is a buyer's request to run one federated job across N datasets.
type FederatedSubmitInput struct {
	AlgorithmID string
	DatasetIDs  []string
	Params      map[string]any
	DPEpsilon   *float64
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
	// Pre-flight every dataset BEFORE creating anything, so we never fan out a
	// partially-valid job. Resolve the active entitlement per dataset up front.
	entByDataset := make(map[string]string, len(in.DatasetIDs))
	for _, ds := range in.DatasetIDs {
		offer, err := s.repo.GetOffer(ctx, ds)
		if err != nil {
			return FederatedJob{}, err
		}
		if !offer.Enabled || !offer.AllowFederated {
			return FederatedJob{}, fmt.Errorf("%w: dataset %s does not allow federated use", ErrOfferDisabled, ds)
		}
		ent, err := s.activeEntitlementFor(ctx, buyerID, ds)
		if err != nil {
			return FederatedJob{}, err
		}
		entByDataset[ds] = ent.ID
	}

	fed, err := s.repo.CreateFederatedJob(ctx, FederatedJob{
		BuyerID: buyerID, AlgorithmID: in.AlgorithmID, DatasetIDs: in.DatasetIDs,
		Mode: ModeFederated, MinParticipants: len(in.DatasetIDs), Params: in.Params, DPEpsilon: in.DPEpsilon,
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
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "compute.federated.submit",
		ResourceType: "compute_federated_job", ResourceID: fed.ID,
		Detail: map[string]any{"datasets": len(in.DatasetIDs), "algorithm_id": in.AlgorithmID}})
	return fed, nil
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

// tryAdvanceFederated is called after each sub-job reaches a terminal state. It
// is idempotent: only the transition that finds the job advanceable proceeds.
// If any sub-job failed/rejected, the whole federated job fails (refunding all).
// When all sub-jobs are released, it triggers aggregation.
func (s *Service) tryAdvanceFederated(ctx context.Context, fedID string) {
	fed, err := s.repo.GetFederatedJob(ctx, fedID)
	if err != nil || fed.Status != FedFanout {
		return
	}
	subs, err := s.repo.ListSubJobs(ctx, fedID)
	if err != nil {
		return
	}
	released := 0
	for _, j := range subs {
		switch j.Status {
		case JobReleased:
			released++
		case JobFailed, JobRejected, JobCanceled:
			// First caller to flip fanout→failed wins and refunds everyone once.
			if _, ferr := s.repo.FailFederated(ctx, fedID, "subjob_"+j.Status); ferr == nil {
				s.refundFederated(ctx, subs)
			}
			return
		}
	}
	if released < len(subs) {
		return // not all sub-jobs done yet
	}
	// All released → claim the aggregation step (only one goroutine wins).
	if _, err := s.repo.TransitionFederated(ctx, fedID, FedFanout, FedAggregating); err != nil {
		return
	}
	s.aggregateAndRelease(ctx, fed, subs)
}

// aggregateAndRelease reads each sub-job's local params, runs the aggregator,
// gates the joint output, and releases it. On any error it fails the federated
// job and refunds all participants.
func (s *Service) aggregateAndRelease(ctx context.Context, fed FederatedJob, subs []Job) {
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
	joint, err := s.aggregator.Aggregate(partials)
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
	s.audit.Record(ctx, audit.Entry{Action: "compute.federated.release", ResourceType: "compute_federated_job",
		ResourceID: fed.ID, Detail: map[string]any{"participants": len(partials), "output_bytes": size}})
}

// failFederatedWithRefund fails the job (idempotently) and refunds all sub-jobs once.
func (s *Service) failFederatedWithRefund(ctx context.Context, fedID, code string, subs []Job) {
	if _, err := s.repo.FailFederated(ctx, fedID, code); err == nil {
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
