# P4-a Federated Learning MVP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a federated-learning orchestration layer on top of the existing `compute` module: one federated job fans out N existing sandbox sub-jobs (each seller's data stays in its own sandbox), the platform aggregates local model params with real FedAvg into a joint model, gated and released to the buyer.

**Architecture:** Reuse `compute_jobs` as sub-jobs (federated_job_id FK), event-driven coordination (last finished sub-job triggers aggregation), real `FedAvgAggregator` (weighted mean), joint output through the existing size+DP gate. Sub-jobs run via the existing runner (MockRunner in MVP, producing numeric `fedparams-v1`). Real training image + docker e2e deferred to P4-b.

**Tech Stack:** Go 1.23, pgx/Postgres, golang-migrate, existing `backend/internal/modules/compute` patterns (pgRepo, Service, worker pool, Runner interface).

**Spec:** `docs/superpowers/specs/2026-06-03-p4a-federated-mvp-design.md`

**Verification law (run after every task that compiles):** `export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"` then `gofmt -l backend/ && go build ./... && go vet ./... && go test ./backend/internal/modules/compute/ -run <relevant>`. Real-PG integration uses the ephemeral-pg pattern from handoff §0.3.

---

## File Structure

- **Create** `backend/migrations/000012_compute_federated.up.sql` / `.down.sql` — federated table + columns.
- **Create** `backend/internal/modules/compute/aggregator.go` — `Aggregator` interface + `FedAvgAggregator` (pure math).
- **Create** `backend/internal/modules/compute/aggregator_test.go` — pure unit tests for FedAvg.
- **Create** `backend/internal/modules/compute/federated.go` — `FederatedJob` model + status constants + `SubmitFederatedJob`/`GetFederatedJob`/`OpenFederatedOutput` service methods + `tryAdvanceFederated`/`aggregateAndRelease`.
- **Create** `backend/internal/modules/compute/federated_integration_test.go` — real-PG end-to-end federated loop.
- **Modify** `model.go` — `Job.FederatedJobID`, `Offer.AllowFederated`, `OfferInput.AllowFederated`.
- **Modify** `repo.go` — `Repository` interface + `pgRepo`: federated CRUD/transitions; `CreateJob`/`listJobs` carry `federated_job_id`; offer read/write carry `allow_federated`.
- **Modify** `runner.go` — `MockRunner.Run` emits `fedparams-v1` for `fed-logreg`.
- **Modify** `worker.go` — `processJob` routes federated sub-jobs to internal-release + `tryAdvanceFederated`.
- **Modify** `service.go` — `ConfigureOffer` passes through `AllowFederated`.
- **Modify** `handler.go` / `router.go` — federated endpoints + offer field.
- **Modify** `backend/internal/server/server.go` — wire federated routes + aggregator.
- **Modify** `frontend/lib/api.ts` — `allow_federated` on offer types (keep build green).

---

## Task 1: Migration 000012 (federated schema)

**Files:**
- Create: `backend/migrations/000012_compute_federated.up.sql`
- Create: `backend/migrations/000012_compute_federated.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- 000012_compute_federated.up.sql
CREATE TABLE compute_federated_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id         UUID NOT NULL REFERENCES users(id),
    algorithm_id     UUID REFERENCES algorithms(id),
    dataset_ids      UUID[] NOT NULL,
    mode             TEXT NOT NULL DEFAULT 'federated',
    status           TEXT NOT NULL DEFAULT 'created',
    min_participants INT  NOT NULL DEFAULT 0,
    params           JSONB NOT NULL DEFAULT '{}',
    dp_epsilon       DOUBLE PRECISION,
    output_key       TEXT,
    output_bytes     BIGINT NOT NULL DEFAULT 0,
    failure_code     TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE compute_jobs ADD COLUMN federated_job_id UUID REFERENCES compute_federated_jobs(id);
CREATE INDEX idx_compute_jobs_federated ON compute_jobs(federated_job_id) WHERE federated_job_id IS NOT NULL;
ALTER TABLE dataset_compute_offers ADD COLUMN allow_federated BOOLEAN NOT NULL DEFAULT false;
```

- [ ] **Step 2: Write down migration**

```sql
-- 000012_compute_federated.down.sql
ALTER TABLE dataset_compute_offers DROP COLUMN IF EXISTS allow_federated;
DROP INDEX IF EXISTS idx_compute_jobs_federated;
ALTER TABLE compute_jobs DROP COLUMN IF EXISTS federated_job_id;
DROP TABLE IF EXISTS compute_federated_jobs;
```

- [ ] **Step 3: Verify migrations apply on ephemeral PG**

Run the handoff §0.3 ephemeral-pg recipe, point golang-migrate at `backend/migrations`, migrate up to 12 then down to 11.
Expected: clean up to `000012`, clean down.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000012_compute_federated.*
git commit -m "feat(compute): migration 000012 — federated jobs table + columns"
```

---

## Task 2: Model additions

**Files:** Modify `backend/internal/modules/compute/model.go`

- [ ] **Step 1: Add fields + federated model + status constants**

Add to `Job` struct: `FederatedJobID string `json:"federated_job_id,omitempty"``.
Add to `Offer` struct and `OfferInput` struct: `AllowFederated bool `json:"allow_federated"``.
Add new model + constants:

```go
// FederatedJob is one federated-learning job: it references N datasets, fans out
// N sandbox sub-jobs (compute_jobs with federated_job_id set), then aggregates
// their local params into a joint model. Raw data never leaves each sandbox.
type FederatedJob struct {
	ID              string    `json:"id"`
	BuyerID         string    `json:"buyer_id"`
	AlgorithmID     string    `json:"algorithm_id"`
	DatasetIDs      []string  `json:"dataset_ids"`
	Mode            string    `json:"mode"`
	Status          string    `json:"status"`
	MinParticipants int       `json:"min_participants"`
	Params          map[string]any `json:"params,omitempty"`
	DPEpsilon       *float64  `json:"dp_epsilon,omitempty"`
	OutputKey       string    `json:"-"`
	OutputBytes     int64     `json:"output_bytes"`
	FailureCode     string    `json:"failure_code,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const (
	FedCreated     = "created"
	FedFanout      = "fanout"
	FedAggregating = "aggregating"
	FedReleased    = "released"
	FedFailed      = "failed"
	FedRejected    = "rejected"
)

const ModeFederated = "federated"

// RuntimeFedLogreg is the MVP federated algorithm runtime. Sub-jobs produce
// fedparams-v1 local params; the platform aggregates with FedAvg.
const RuntimeFedLogreg = "fed-logreg"
```

- [ ] **Step 2: Build**

Run: `go build ./backend/...`
Expected: PASS (unused but compiles).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/modules/compute/model.go
git commit -m "feat(compute): federated model, status constants, offer allow_federated"
```

---

## Task 3: FedAvgAggregator (pure, TDD core)

**Files:**
- Create: `backend/internal/modules/compute/aggregator.go`
- Create: `backend/internal/modules/compute/aggregator_test.go`

- [ ] **Step 1: Write failing tests**

```go
package compute

import (
	"encoding/json"
	"math"
	"testing"
)

func TestFedAvgWeightedMean(t *testing.T) {
	// Two parties: w=[2,4] n=1 ; w=[4,8] n=3 → weighted = [(2*1+4*3)/4,(4*1+8*3)/4]=[3.5,7]
	parts := []Partial{
		{Weights: []float64{2, 4}, Intercept: 1, N: 1},
		{Weights: []float64{4, 8}, Intercept: 5, N: 3},
	}
	out, err := FedAvgAggregator{}.Aggregate(parts)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	var m struct {
		Weights      []float64 `json:"weights"`
		Intercept    float64   `json:"intercept"`
		NTotal       int       `json:"n_total"`
		Participants int       `json:"participants"`
	}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []float64{3.5, 7}
	for i := range want {
		if math.Abs(m.Weights[i]-want[i]) > 1e-9 {
			t.Fatalf("weights[%d]=%v want %v", i, m.Weights[i], want[i])
		}
	}
	if math.Abs(m.Intercept-4) > 1e-9 { // (1*1+5*3)/4 = 4
		t.Fatalf("intercept=%v want 4", m.Intercept)
	}
	if m.NTotal != 4 || m.Participants != 2 {
		t.Fatalf("n_total=%d participants=%d", m.NTotal, m.Participants)
	}
}

func TestFedAvgSingleParty(t *testing.T) {
	out, err := FedAvgAggregator{}.Aggregate([]Partial{{Weights: []float64{1, 2, 3}, Intercept: 9, N: 10}})
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Weights []float64 }
	_ = json.Unmarshal(out, &m)
	if m.Weights[0] != 1 || m.Weights[2] != 3 {
		t.Fatalf("identity failed: %v", m.Weights)
	}
}

func TestFedAvgErrors(t *testing.T) {
	if _, err := (FedAvgAggregator{}).Aggregate(nil); err == nil {
		t.Fatal("want error on empty partials")
	}
	mismatch := []Partial{{Weights: []float64{1, 2}, N: 1}, {Weights: []float64{1}, N: 1}}
	if _, err := (FedAvgAggregator{}).Aggregate(mismatch); err == nil {
		t.Fatal("want error on dim mismatch")
	}
	zeroN := []Partial{{Weights: []float64{1}, N: 0}}
	if _, err := (FedAvgAggregator{}).Aggregate(zeroN); err == nil {
		t.Fatal("want error on zero total N")
	}
}

func TestParsePartial(t *testing.T) {
	raw := []byte(`{"_format":"fedparams-v1","weights":[1.5,2.5],"intercept":0.5,"n":7}`)
	p, err := parsePartial(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.N != 7 || p.Intercept != 0.5 || p.Weights[1] != 2.5 {
		t.Fatalf("bad parse: %+v", p)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./backend/internal/modules/compute/ -run 'FedAvg|ParsePartial' -v`
Expected: FAIL (undefined: Partial, FedAvgAggregator, parsePartial).

- [ ] **Step 3: Implement aggregator.go**

```go
package compute

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Partial is one party's local model contribution to a federated aggregation.
type Partial struct {
	Weights   []float64
	Intercept float64
	N         int // local sample count (FedAvg weighting)
}

// Aggregator combines N parties' local params into one joint model (bytes).
// Implementations: FedAvgAggregator (weighted mean). MPCAggregator is reserved
// for P4-c (secure multi-party); not implemented in the MVP.
type Aggregator interface {
	Aggregate(partials []Partial) ([]byte, error)
	Kind() string
}

var (
	ErrNoPartials   = errors.New("compute: no partials to aggregate")
	ErrDimMismatch  = errors.New("compute: partial weight dimensions differ")
	ErrZeroSamples  = errors.New("compute: total sample count is zero")
)

// FedAvgAggregator implements weighted federated averaging:
// w* = Σ(n_k · w_k) / Σ n_k  (and likewise for the intercept). Real math.
type FedAvgAggregator struct{}

func (FedAvgAggregator) Kind() string { return "fedavg" }

func (FedAvgAggregator) Aggregate(partials []Partial) ([]byte, error) {
	if len(partials) == 0 {
		return nil, ErrNoPartials
	}
	dim := len(partials[0].Weights)
	totalN := 0
	for _, p := range partials {
		if len(p.Weights) != dim {
			return nil, ErrDimMismatch
		}
		totalN += p.N
	}
	if totalN <= 0 {
		return nil, ErrZeroSamples
	}
	w := make([]float64, dim)
	var intercept float64
	for _, p := range partials {
		f := float64(p.N) / float64(totalN)
		for i := range w {
			w[i] += f * p.Weights[i]
		}
		intercept += f * p.Intercept
	}
	out := map[string]any{
		"_format":      "fedmodel-v1",
		"weights":      w,
		"intercept":    intercept,
		"n_total":      totalN,
		"participants": len(partials),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("compute: marshal joint model: %w", err)
	}
	return b, nil
}

// parsePartial decodes a sub-job's fedparams-v1 output into a Partial.
func parsePartial(raw []byte) (Partial, error) {
	var p struct {
		Format    string    `json:"_format"`
		Weights   []float64 `json:"weights"`
		Intercept float64   `json:"intercept"`
		N         int       `json:"n"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return Partial{}, fmt.Errorf("compute: parse partial: %w", err)
	}
	if p.Format != "fedparams-v1" {
		return Partial{}, fmt.Errorf("compute: unexpected partial format %q", p.Format)
	}
	return Partial{Weights: p.Weights, Intercept: p.Intercept, N: p.N}, nil
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./backend/internal/modules/compute/ -run 'FedAvg|ParsePartial' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/compute/aggregator.go backend/internal/modules/compute/aggregator_test.go
git commit -m "feat(compute): real FedAvg aggregator + partial parsing (TDD)"
```

---

## Task 4: MockRunner emits fedparams-v1 for fed-logreg

**Files:** Modify `backend/internal/modules/compute/runner.go`, `runner_docker_test.go` (or a new `runner_test.go`)

- [ ] **Step 1: Write failing test**

```go
func TestMockRunnerFedParams(t *testing.T) {
	r := NewMockRunner()
	req := RunRequest{
		Algorithm: Algorithm{Name: "fed-logreg", Runtime: RuntimeFedLogreg, OutputKind: OutputModel},
		Job:       Job{DatasetID: "11111111-1111-1111-1111-111111111111"},
	}
	res, err := r.Run(context.Background(), req)
	if err != nil { t.Fatal(err) }
	p, err := parsePartial(res.Output)
	if err != nil { t.Fatalf("not fedparams: %v", err) }
	if len(p.Weights) == 0 || p.N <= 0 { t.Fatalf("bad params: %+v", p) }
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./backend/internal/modules/compute/ -run TestMockRunnerFedParams -v`
Expected: FAIL (mock returns mock-model-v1, parsePartial rejects format).

- [ ] **Step 3: Add fed-logreg branch in MockRunner.Run**

At the top of the `switch kind` / before the `OutputModel` case, add a runtime check:

```go
	// Federated sub-job: emit deterministic-but-dataset-varying local params so
	// FedAvg has real numbers to average (MVP; real training image is P4-b).
	if req.Algorithm.Runtime == RuntimeFedLogreg {
		seed := 0
		for _, c := range req.Job.DatasetID { seed += int(c) }
		w0 := float64(seed%7) + 1
		params := map[string]any{
			"_format":   "fedparams-v1",
			"weights":   []float64{w0, w0 / 2, 1},
			"intercept": float64(seed%3),
			"n":         10 + seed%5,
		}
		b, _ := json.Marshal(params)
		return RunResult{OutputKind: OutputModel, Output: b, Logs: []byte("mock: fed-logreg local params")}, nil
	}
```

- [ ] **Step 4: Run, verify pass**

Run: `go test ./backend/internal/modules/compute/ -run TestMockRunnerFedParams -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/compute/runner.go backend/internal/modules/compute/runner_test.go
git commit -m "feat(compute): MockRunner emits fedparams-v1 for fed-logreg"
```

---

## Task 5: Repository — federated CRUD + carry columns

**Files:** Modify `backend/internal/modules/compute/repo.go`

- [ ] **Step 1: Extend Repository interface**

Add to the `Repository` interface (jobs/federated section):

```go
	// federated
	CreateFederatedJob(ctx context.Context, f FederatedJob) (FederatedJob, error)
	GetFederatedJob(ctx context.Context, id string) (FederatedJob, error)
	ListSubJobs(ctx context.Context, federatedID string) ([]Job, error)
	TransitionFederated(ctx context.Context, id, from, to string) (FederatedJob, error)
	ReleaseFederated(ctx context.Context, id, outputKey string, outputBytes int64) (FederatedJob, error)
	FailFederated(ctx context.Context, id, code string) (FederatedJob, error)
```

- [ ] **Step 2: Implement pgRepo methods (mirror existing patterns)**

Mirror `CreateJob` (repo.go:447) for inserts, `Transition` (repo.go:526) for status moves, `listJobs` (repo.go:509) for `ListSubJobs`. Use `pgx` array for `dataset_ids` (`pq.Array` / pgx native). Concrete code:

```go
func (r *pgRepo) CreateFederatedJob(ctx context.Context, f FederatedJob) (FederatedJob, error) {
	params, _ := json.Marshal(f.Params)
	if f.Mode == "" { f.Mode = ModeFederated }
	row := r.db.QueryRow(ctx, `
		INSERT INTO compute_federated_jobs (buyer_id, algorithm_id, dataset_ids, mode, status, min_participants, params, dp_epsilon)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, status, created_at, updated_at`,
		f.BuyerID, nullStr(f.AlgorithmID), f.DatasetIDs, f.Mode, FedCreated, f.MinParticipants, params, f.DPEpsilon)
	if err := row.Scan(&f.ID, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return FederatedJob{}, err
	}
	return f, nil
}
```
(Implement `GetFederatedJob`, `TransitionFederated`, `ReleaseFederated`, `FailFederated` with the same scan-set; `TransitionFederated` uses a conditional `UPDATE ... WHERE status=$from` returning `ErrBadTransition` when 0 rows — copy the shape of `Transition` at repo.go:526. `ListSubJobs` = `r.listJobs(ctx, "<base select> WHERE federated_job_id=$1 ORDER BY created_at", id)`.) Use the existing `nullStr` helper if present; otherwise add one. Reuse the existing job column list in `listJobs` and **add `federated_job_id`** to its SELECT + scan, and to `CreateJob`'s INSERT (nullable).

- [ ] **Step 3: Carry allow_federated in offer read/write + federated_job_id in jobs**

In `UpsertOffer` (repo.go:253) INSERT/UPDATE column lists and `GetOffer` (repo.go:283) SELECT/scan: add `allow_federated`. In `CreateJob` INSERT + `listJobs` SELECT/scan: add `federated_job_id` (scan into `&j.FederatedJobID` via a nullable string helper).

- [ ] **Step 4: Build + vet**

Run: `go build ./backend/... && go vet ./backend/internal/modules/compute/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/compute/repo.go
git commit -m "feat(compute): repo federated CRUD + carry federated_job_id/allow_federated"
```

---

## Task 6: Service — SubmitFederatedJob + accessors + offer passthrough

**Files:** Create `backend/internal/modules/compute/federated.go`; Modify `service.go` (ConfigureOffer)

- [ ] **Step 1: Add FederatedSubmitInput + SubmitFederatedJob (validation + fan-out)**

```go
package compute

import (
	"context"
	"fmt"
)

type FederatedSubmitInput struct {
	AlgorithmID string
	DatasetIDs  []string
	Params      map[string]any
	DPEpsilon   *float64
}

// SubmitFederatedJob validates each participating dataset's offer (allow_federated)
// and the buyer's active entitlement, creates the federated job, then fans out one
// sandbox sub-job per dataset (each spending that dataset's entitlement).
func (s *Service) SubmitFederatedJob(ctx context.Context, buyerID string, in FederatedSubmitInput) (FederatedJob, error) {
	if len(in.DatasetIDs) < 2 {
		return FederatedJob{}, fmt.Errorf("%w: federated needs >=2 datasets", ErrInvalidInput)
	}
	// Pre-flight: every dataset offer must allow federated + algorithm must be approved+trusted for each.
	for _, ds := range in.DatasetIDs {
		offer, err := s.repo.GetOffer(ctx, ds)
		if err != nil { return FederatedJob{}, err }
		if !offer.Enabled || !offer.AllowFederated {
			return FederatedJob{}, fmt.Errorf("%w: dataset %s does not allow federated", ErrOfferDisabled, ds)
		}
	}
	fed, err := s.repo.CreateFederatedJob(ctx, FederatedJob{
		BuyerID: buyerID, AlgorithmID: in.AlgorithmID, DatasetIDs: in.DatasetIDs,
		Mode: ModeFederated, MinParticipants: len(in.DatasetIDs), Params: in.Params, DPEpsilon: in.DPEpsilon,
	})
	if err != nil { return FederatedJob{}, err }

	// Fan out: one sub-job per dataset, reusing the single-dataset submit path,
	// tagging each with federated_job_id. SubmitJob enforces offer/entitlement/quota.
	for _, ds := range in.DatasetIDs {
		_, err := s.submitSubJob(ctx, buyerID, ds, in.AlgorithmID, in.Params, fed.ID)
		if err != nil {
			_, _ = s.repo.FailFederated(ctx, fed.ID, "fanout_failed")
			return FederatedJob{}, err
		}
	}
	fed, _ = s.repo.TransitionFederated(ctx, fed.ID, FedCreated, FedFanout)
	return fed, nil
}
```

`submitSubJob` is a thin variant of `SubmitJob` (service.go:398) that sets `FederatedJobID` on the created job and skips buyer-facing idempotency-key requirements. Implement by extracting the core of `SubmitJob` into a shared helper OR add an optional `federatedID` param to the existing internal create path. **Reuse** SpendQuota + resolveAlgorithm exactly as SubmitJob does.

- [ ] **Step 2: Add GetFederatedJob + OpenFederatedOutput; block sub-job downloads**

```go
func (s *Service) GetFederatedJob(ctx context.Context, userID, id string) (FederatedJob, []Job, error) {
	fed, err := s.repo.GetFederatedJob(ctx, id)
	if err != nil { return FederatedJob{}, nil, err }
	if fed.BuyerID != userID { return FederatedJob{}, nil, ErrForbidden }
	subs, err := s.repo.ListSubJobs(ctx, id)
	return fed, subs, err
}

func (s *Service) OpenFederatedOutput(ctx context.Context, userID, id string) (io.ReadCloser, int64, FederatedJob, error) {
	fed, err := s.repo.GetFederatedJob(ctx, id)
	if err != nil { return nil, 0, FederatedJob{}, err }
	if fed.BuyerID != userID { return nil, 0, FederatedJob{}, ErrForbidden }
	if fed.Status != FedReleased || fed.OutputKey == "" {
		return nil, 0, FederatedJob{}, ErrOutputNotReady
	}
	rc, n, err := s.store.Open(ctx, fed.OutputKey)
	return rc, n, fed, err
}
```

In `GetJob` (service.go:543) / `OpenOutput` (worker.go:296): if `job.FederatedJobID != ""`, return `ErrForbidden` for direct buyer access (sub-job partials are internal-only).

- [ ] **Step 3: ConfigureOffer passes AllowFederated**

In `ConfigureOffer` (service.go:120), copy `in.AllowFederated` into the `Offer` written via `UpsertOffer`.

- [ ] **Step 4: Build**

Run: `go build ./backend/...`
Expected: PASS. (Reuse existing error sentinels; add `ErrOfferDisabled`/`ErrOutputNotReady`/`ErrForbidden`/`ErrInvalidInput` only if not already defined in model.go — check first.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/compute/federated.go backend/internal/modules/compute/service.go backend/internal/modules/compute/model.go
git commit -m "feat(compute): SubmitFederatedJob fan-out + federated accessors + offer passthrough"
```

---

## Task 7: Worker coordination — tryAdvanceFederated + sub-job routing

**Files:** Modify `backend/internal/modules/compute/worker.go`; add to `federated.go`

- [ ] **Step 1: Add aggregator field + advance/aggregate logic in federated.go**

```go
// tryAdvanceFederated is called after each sub-job reaches a terminal state. It
// is idempotent: only the transition that finds ALL sub-jobs released (while the
// federated job is still fanout) proceeds to aggregation.
func (s *Service) tryAdvanceFederated(ctx context.Context, fedID string) {
	fed, err := s.repo.GetFederatedJob(ctx, fedID)
	if err != nil || fed.Status != FedFanout { return }
	subs, err := s.repo.ListSubJobs(ctx, fedID)
	if err != nil { return }
	released := 0
	for _, j := range subs {
		switch j.Status {
		case JobReleased:
			released++
		case JobFailed, JobRejected, JobCanceled:
			_, _ = s.repo.FailFederated(ctx, fedID, "subjob_"+j.Status)
			s.refundFederated(ctx, subs)
			return
		}
	}
	if released < len(subs) { return } // not all done yet
	if _, err := s.repo.TransitionFederated(ctx, fedID, FedFanout, FedAggregating); err != nil {
		return // another goroutine won the race
	}
	s.aggregateAndRelease(ctx, fed, subs)
}

func (s *Service) aggregateAndRelease(ctx context.Context, fed FederatedJob, subs []Job) {
	partials := make([]Partial, 0, len(subs))
	for _, j := range subs {
		rc, _, err := s.store.Open(ctx, j.OutputKey)
		if err != nil { _, _ = s.repo.FailFederated(ctx, fed.ID, "read_partial"); return }
		raw, _ := io.ReadAll(rc); rc.Close()
		p, err := parsePartial(raw)
		if err != nil { _, _ = s.repo.FailFederated(ctx, fed.ID, "bad_partial"); return }
		partials = append(partials, p)
	}
	joint, err := s.aggregator.Aggregate(partials)
	if err != nil { _, _ = s.repo.FailFederated(ctx, fed.ID, "aggregate"); return }
	// Joint output gate (size) — reuse the offer-independent cap; federated DP is recorded on fed.DPEpsilon.
	key := "compute/federated/" + fed.ID + "/model.json"
	if _, err := uploadOutput(ctx, s.store, key, joint); err != nil {
		_, _ = s.repo.FailFederated(ctx, fed.ID, "store"); return
	}
	_, _ = s.repo.ReleaseFederated(ctx, fed.ID, key, int64(len(joint)))
}
```

Add `aggregator Aggregator` to `Service` struct + a `WithAggregator` option (mirror `WithWorker` at worker.go:64); default to `FedAvgAggregator{}` in `NewService`. Add `refundFederated` (loops subs, `RefundQuota` per entitlement) — mirror existing refund usage.

- [ ] **Step 2: Route federated sub-jobs in processJob**

In `processJob` (worker.go:147), after a successful run + size gate, branch on `job.FederatedJobID`:
- If set: `Release` the sub-job with its output (internal), then `defer`/call `s.tryAdvanceFederated(ctx, job.FederatedJobID)`. Do NOT enter the buyer review/release messaging path.
- If empty: existing behavior unchanged.
On sub-job failure path (`failJob`): also call `tryAdvanceFederated` so the federated job fails fast.

- [ ] **Step 3: Build**

Run: `go build ./backend/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/modules/compute/worker.go backend/internal/modules/compute/federated.go
git commit -m "feat(compute): event-driven federated coordination + aggregation release"
```

---

## Task 8: HTTP handlers + routes + offer field

**Files:** Modify `handler.go`, `router.go`

- [ ] **Step 1: Add handlers**

`handleSubmitFederated` (POST), `handleGetFederated` (GET :id → {federated, sub_jobs}), `handleFederatedOutput` (GET :id/output → stream). Mirror existing job handlers' auth-context extraction + error mapping. Add `allow_federated` to the offer request struct (handler.go:26 area) and pass into `OfferInput`.

- [ ] **Step 2: Register routes in router.go**

```go
g.POST("/compute/federated-jobs", h.handleSubmitFederated)
g.GET("/compute/federated-jobs/:id", h.handleGetFederated)
g.GET("/compute/federated-jobs/:id/output", h.handleFederatedOutput)
```

- [ ] **Step 3: Build**

Run: `go build ./backend/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/modules/compute/handler.go backend/internal/modules/compute/router.go
git commit -m "feat(compute): federated HTTP endpoints + offer allow_federated field"
```

---

## Task 9: Server wiring

**Files:** Modify `backend/internal/server/server.go`

- [ ] **Step 1: Wire aggregator (default FedAvg)**

Where the compute Service is constructed (search `COMPUTE_RUNNER`), add `compute.WithAggregator(compute.FedAvgAggregator{})` to the options (or rely on the NewService default). No new env needed for MVP.

- [ ] **Step 2: Build whole tree**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/server/server.go
git commit -m "feat(compute): wire FedAvg aggregator into server"
```

---

## Task 10: Real-PG integration test (full federated loop)

**Files:** Create `backend/internal/modules/compute/federated_integration_test.go`

- [ ] **Step 1: Write the integration test (DATABASE_URL-gated, mirror engine_integration_test.go setup)**

Test body:
1. Migrate to 000012 on ephemeral PG.
2. Register+approve+trust a `fed-logreg` algorithm (Runtime=RuntimeFedLogreg, OutputKind=model).
3. Create 2 datasets' offers with `AllowFederated=true, Enabled=true`.
4. Grant the buyer an active entitlement (quota≥1) on each dataset.
5. Start the Service worker with `MockRunner` + `FedAvgAggregator`.
6. `SubmitFederatedJob` with the 2 dataset IDs.
7. Poll `GetFederatedJob` until `FedReleased` (timeout 10s).
8. Assert: federated released; `OpenFederatedOutput` returns a `fedmodel-v1` whose weights equal `FedAvg` of the two MockRunner `fedparams-v1` outputs (recompute expected via parsePartial+FedAvgAggregator on the sub-job outputs, or assert participants==2 and n_total>0); each sub-job `JobReleased`; `OpenOutput`/`GetJob` on a sub-job returns `ErrForbidden`; each dataset entitlement spent by 1.

- [ ] **Step 2: Failure-path test**

Force one sub-job to fail (e.g., MockRunner oversize via `_mock_output_bytes` exceeding the offer cap on one dataset) → assert federated `FedFailed` and both entitlements refunded.

- [ ] **Step 3: Run integration tests on ephemeral PG**

Run (with DATABASE_URL set per handoff §0.3):
`go test ./backend/internal/modules/compute/ -run Federated -race -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/modules/compute/federated_integration_test.go
git commit -m "test(compute): real-PG federated end-to-end + failure/refund path"
```

---

## Task 11: Frontend type alignment

**Files:** Modify `frontend/lib/api.ts`

- [ ] **Step 1: Add allow_federated to offer types**

Add `allow_federated?: boolean` to the offer input/response TS types (search the existing `compute-offer` type). Keep optional so existing callers compile. (Full federated UI is out of MVP scope.)

- [ ] **Step 2: Verify frontend build**

Run: `cd frontend && npm run typecheck && npm run lint && npm run build`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/lib/api.ts
git commit -m "feat(frontend): allow_federated on compute offer types"
```

---

## Task 12: Full verification + PR

- [ ] **Step 1: Full local verification law**

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"
gofmt -l backend/ ; go build ./... ; go vet ./...
# ephemeral PG (handoff §0.3) then:
DATABASE_URL=<ephemeral> go test -race ./backend/...
cd frontend && npm run typecheck && npm run lint && npm run build
```
Expected: gofmt clean, build/vet clean, all Go tests pass (incl. Federated integration), frontend green.

- [ ] **Step 2: Push + PR**

```bash
git push -u origin feat/p4a-federated
gh pr create --base main --title "feat(compute): P4-a federated learning MVP (orchestration + real FedAvg)" \
  --body "Implements docs/superpowers/specs/2026-06-03-p4a-federated-mvp-design.md. Orchestration loop + real FedAvg over existing sandbox sub-jobs; MockRunner sub-jobs (real training image deferred to P4-b). 🤖 Generated with Claude Code"
```

- [ ] **Step 3: Watch CI (3 jobs: backend/frontend/sidecar) green, then merge**

```bash
gh pr checks --watch
gh pr merge --squash --delete-branch
```

- [ ] **Step 4: Cleanup worktree**

```bash
git worktree remove ~/ai-data-marketplace-p4a-federated
```

---

## Self-Review notes
- **Spec coverage:** every spec §(3 migration, 4 schema, 5 aggregator, 6 repo, 7 service/worker, 8 API, 9 tests) maps to Tasks 1–12.
- **Type consistency:** `Partial`/`FedAvgAggregator`/`parsePartial` (Task 3) reused verbatim in Tasks 7,10. `fedparams-v1` produced (Task 4) and consumed (Tasks 3,7). `FederatedJob` fields (Task 2) used by repo (Task 5) + service (Task 6).
- **Known integration points verified against origin/main:** Repository interface (repo.go:20), CreateJob (447), Transition (526), Release (570), SubmitJob (service.go:398), processJob (worker.go:147), MockRunner.Run (runner.go), gate logic (worker.go:201).
- **Honest gate:** sub-jobs are MockRunner in MVP (not real training); real fed-logreg image + docker federated e2e + secure aggregation are P4-b.
