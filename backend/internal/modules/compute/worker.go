package compute

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/metrics"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// stageData copies a dataset object from storage to a fresh local dir (as
// input.csv) so a sandbox runner can mount it read-only. Returns the dir and a
// cleanup func. This runs on the runner host BEFORE the algorithm container's
// network is severed (design §18.3).
func stageData(ctx context.Context, store storage.Storage, key string) (string, func(), error) {
	rc, _, err := store.Open(ctx, key)
	if err != nil {
		return "", nil, fmt.Errorf("open dataset object: %w", err)
	}
	defer rc.Close()
	dir, err := os.MkdirTemp("", "c2d-data-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	f, err := os.Create(filepath.Join(dir, "input.csv"))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// randSuffix builds a per-process runner id (pid + start nanos) for lease
// ownership. Uniqueness across processes is enough; within a process all
// workers share one id (they coordinate via the DB claim).
func randSuffix() string { return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()) }

// WithWorker enables in-process execution: `workers` goroutines drain a queue,
// each claiming a job (lease), running it through the Runner, applying the
// output gate, storing the output, and releasing — mirroring the dataset
// quality worker. A background sweep reclaims crashed (lease-expired) jobs. Call
// Close on shutdown to drain in-flight work.
//
// The interface is queue-agnostic: swap this in-process worker for a separate
// runner service (claiming over HTTP/mTLS, design §18) without touching the
// business logic — the job lifecycle is the same DB state machine either way.
func WithWorker(runner Runner, store storage.Storage, data DataKeyResolver, workers, buffer int) Option {
	return func(s *Service) {
		if workers < 1 {
			workers = 1
		}
		if buffer < 1 {
			buffer = 1
		}
		s.runner = runner
		s.store = store
		s.data = data
		s.runnerID = "inproc-" + randSuffix()
		s.qCh = make(chan string, buffer)
		s.stopSweep = make(chan struct{})

		for i := 0; i < workers; i++ {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				for jobID := range s.qCh {
					s.processJob(context.Background(), jobID)
				}
			}()
		}
		// Reclaim any jobs left "running" by a previous crashed process, then
		// sweep periodically.
		if n, err := s.repo.ReclaimStaleLeases(context.Background(), s.maxAttempts); err != nil {
			slog.Error("compute: startup lease reclaim failed", "err", err)
		} else {
			metrics.RecordComputeReclaims(n)
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			t := time.NewTicker(time.Duration(s.leaseSecs) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-s.stopSweep:
					return
				case <-t.C:
					if n, err := s.repo.ReclaimStaleLeases(context.Background(), s.maxAttempts); err != nil {
						slog.Error("compute: lease reclaim failed", "err", err)
					} else if n > 0 {
						metrics.RecordComputeReclaims(n)
						slog.Warn("compute: reclaimed stale jobs", "count", n)
					}
				}
			}
		}()
	}
}

// enqueue dispatches a queued job to the worker pool, or runs it inline when no
// worker is configured (tests/determinism). When a separate out-of-process
// runner is used (no in-process worker), the job simply stays queued for it.
func (s *Service) enqueue(jobID string) {
	if s.qCh == nil {
		return
	}
	select {
	case s.qCh <- jobID:
	default:
		// Queue full: leave it queued; the periodic sweep / a future poll picks
		// it up. (P1 buffer is generous; this only trips under heavy backlog.)
		slog.Warn("compute: worker queue full, job left queued", "job_id", jobID)
	}
}

// Close drains in-flight jobs and stops the sweep (no-op if no worker).
func (s *Service) Close() {
	if s.qCh == nil {
		return
	}
	close(s.stopSweep)
	close(s.qCh)
	s.wg.Wait()
}

// processJob runs one job through the pipeline: claim (lease) → run → output
// gate (size) → store → DP ledger → release. Failures map to Fail (execution
// error) or Reject (gate). The dataset is read-only; the algorithm never names
// the output (design §7.4).
func (s *Service) processJob(ctx context.Context, jobID string) {
	job, err := s.repo.ClaimJob(ctx, jobID, s.runnerID, s.leaseSecs)
	if err != nil {
		// Already claimed/terminal — not an error for this worker.
		return
	}
	start := time.Now() // run-time clock for metrics
	algo, err := s.repo.GetAlgorithm(ctx, job.AlgorithmID)
	if err != nil {
		s.failJob(ctx, jobID, "algorithm_unavailable")
		return
	}
	offer, err := s.repo.GetOffer(ctx, job.DatasetID)
	if err != nil {
		s.failJob(ctx, jobID, "offer_unavailable")
		return
	}
	dataKey := ""
	if s.data != nil {
		dataKey, _ = s.data.CurrentObjectKey(ctx, job.DatasetID) // best-effort; mock ignores
	}

	// Stage the dataset to a local read-only dir for runners that mount real
	// data (docker/gVisor/TEE). The host pulls + verifies the data BEFORE the
	// algorithm container has its network severed (design §18.3). The mock
	// runner needs nothing staged.
	dataPath := ""
	if s.runner.NeedsStagedData() {
		if dataKey == "" {
			s.failJob(ctx, jobID, "dataset_object_missing")
			return
		}
		dir, cleanup, serr := stageData(ctx, s.store, dataKey)
		if serr != nil {
			slog.Error("compute: data staging failed", "job_id", jobID, "err", serr)
			s.failJob(ctx, jobID, "data_staging_error")
			return
		}
		defer cleanup()
		dataPath = dir
	}

	res, err := s.runner.Run(ctx, RunRequest{
		Job: job, Algorithm: algo, DataKey: dataKey, DataPath: dataPath,
		Params:         effectiveParams(job),
		MaxOutputBytes: offer.MaxOutputBytes, MaxOutputFiles: offer.MaxOutputFiles,
		MaxRuntimeSecs: offer.MaxRuntimeSecs,
	})
	if err != nil {
		slog.Error("compute: runner failed", "job_id", jobID, "err", err)
		s.failJob(ctx, jobID, "algorithm_error")
		return
	}

	// Output gate: size cap (design §7/§8). A real runner enforces this during
	// write; the mock returns in memory so we check here. A gate rejection
	// refunds the buyer's credit (§21: rejected output isn't billed).
	if offer.MaxOutputBytes > 0 && int64(len(res.Output)) > offer.MaxOutputBytes {
		s.rejectJob(ctx, jobID, job.EntitlementID, "output_exceeds_max_bytes")
		return
	}

	key := outputObjectKey(jobID)
	size, err := uploadOutput(ctx, s.store, key, res.Output)
	if err != nil {
		slog.Error("compute: output store failed", "job_id", jobID, "err", err)
		s.failJob(ctx, jobID, "output_store_error")
		return
	}

	// DP ledger: record the spent epsilon for aggregate/metric outputs (§8).
	if job.DPEpsilon != nil {
		if err := s.repo.SpendDP(ctx, job.DatasetID, job.BuyerID, jobID, *job.DPEpsilon); err != nil {
			slog.Error("compute: dp ledger write failed", "job_id", jobID, "err", err)
		}
	}

	// Logs are returned to the buyer only when the seller opted in; even then
	// they would pass a scrub gate. P1: store nothing by default (design §7.4).
	logsKey := ""

	// High-sensitivity offers park the output for ops human review before it is
	// released to the buyer (design §8 gate ⑤). Otherwise release directly.
	if offer.ReviewOutput {
		if _, err := s.repo.StageForReview(ctx, jobID, key, res.OutputKind, size, logsKey); err != nil {
			slog.Error("compute: stage-for-review failed", "job_id", jobID, "err", err)
			return
		}
		s.audit.Record(ctx, audit.Entry{Action: "compute.job.review_pending", ResourceType: "compute_job",
			ResourceID: jobID, Detail: map[string]any{"output_kind": res.OutputKind, "output_bytes": size}})
		metrics.RecordComputeJob("review_pending")
		metrics.ObserveComputeJobDuration(res.OutputKind, time.Since(start).Seconds())
		return
	}

	released, err := s.repo.Release(ctx, jobID, key, res.OutputKind, size, logsKey)
	if err != nil {
		slog.Error("compute: release failed", "job_id", jobID, "err", err)
		return
	}
	s.audit.Record(ctx, audit.Entry{Action: "compute.job.release", ResourceType: "compute_job",
		ResourceID: jobID, Detail: map[string]any{"output_kind": released.OutputKind, "output_bytes": size}})
	metrics.RecordComputeJob("released")
	metrics.ObserveComputeJobDuration(released.OutputKind, time.Since(start).Seconds())
}

// rejectJob marks a job's output rejected by the gate and refunds the credit
// (rejected output isn't billed — §21).
func (s *Service) rejectJob(ctx context.Context, jobID, entitlementID, reason string) {
	if _, err := s.repo.Reject(ctx, jobID, reason); err != nil {
		slog.Error("compute: reject failed", "job_id", jobID, "err", err)
		return
	}
	if entitlementID != "" {
		if err := s.repo.RefundQuota(ctx, entitlementID); err != nil {
			slog.Error("compute: quota refund on reject failed", "job_id", jobID, "err", err)
		}
	}
	s.audit.Record(ctx, audit.Entry{Action: "compute.job.reject", ResourceType: "compute_job",
		ResourceID: jobID, Detail: map[string]any{"reason": reason}})
	metrics.RecordComputeJob("rejected")
}

func (s *Service) failJob(ctx context.Context, jobID, code string) {
	if _, err := s.repo.Fail(ctx, jobID, code); err != nil {
		slog.Error("compute: fail transition failed", "job_id", jobID, "code", code, "err", err)
		return
	}
	// A platform/runner-side failure should not consume the buyer's credit (§21).
	if job, gerr := s.repo.GetJob(ctx, jobID); gerr == nil {
		if rerr := s.repo.RefundQuota(ctx, job.EntitlementID); rerr != nil {
			slog.Error("compute: quota refund on fail failed", "job_id", jobID, "err", rerr)
		}
	}
	s.audit.Record(ctx, audit.Entry{Action: "compute.job.fail", ResourceType: "compute_job",
		ResourceID: jobID, Detail: map[string]any{"error": code}})
	metrics.RecordComputeJob("failed")
}

// OpenOutput streams a released job's output to its buyer. Returns ErrForbidden
// for non-owners and ErrBadTransition if the job is not released yet.
func (s *Service) OpenOutput(ctx context.Context, userID, jobID string) (io.ReadCloser, int64, Job, error) {
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return nil, 0, Job{}, err
	}
	if job.BuyerID != userID {
		return nil, 0, Job{}, ErrForbidden
	}
	if job.Status != JobReleased || job.OutputKey == "" {
		return nil, 0, Job{}, ErrBadTransition
	}
	if s.store == nil {
		return nil, 0, Job{}, ErrNotFound
	}
	rc, size, err := s.store.Open(ctx, job.OutputKey)
	if err != nil {
		return nil, 0, Job{}, err
	}
	return rc, size, job, nil
}

// uploadOutput stores the output bytes as a single-part object and returns its
// size.
func uploadOutput(ctx context.Context, store storage.Storage, key string, data []byte) (int64, error) {
	uid, err := store.InitMultipart(ctx, key)
	if err != nil {
		return 0, err
	}
	if _, err := store.PutPart(ctx, uid, 1, bytes.NewReader(data)); err != nil {
		_ = store.Abort(ctx, uid)
		return 0, err
	}
	obj, err := store.CompleteMultipart(ctx, uid)
	if err != nil {
		return 0, err
	}
	return obj.Size, nil
}

// effectiveParams merges the buyer's params with platform-injected keys. The DP
// budget (_epsilon) comes from the offer (job.DPEpsilon), and any buyer-supplied
// _epsilon is dropped — so the buyer cannot weaken or disable the noise (§8).
func effectiveParams(j Job) map[string]any {
	p := make(map[string]any, len(j.Params)+1)
	for k, v := range j.Params {
		if k == "_epsilon" {
			continue
		}
		p[k] = v
	}
	if j.DPEpsilon != nil {
		p["_epsilon"] = *j.DPEpsilon
	}
	return p
}
