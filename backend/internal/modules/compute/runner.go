package compute

import (
	"context"
	"encoding/json"
	"fmt"
)

// RunRequest is everything a runner needs to execute one job in isolation. The
// dataset is provided by reference (object key); a real isolated runner stages
// it read-only and then severs the network before the algorithm runs (design
// §18.3). The mock runner ignores the data.
type RunRequest struct {
	Job            Job
	Algorithm      Algorithm
	DataKey        string         // dataset object-storage key (read-only input)
	DataPath       string         // local dir holding the staged dataset (set by the worker for runners that NeedStagedData)
	Params         map[string]any // EFFECTIVE params handed to the algorithm = buyer params + platform-injected keys (e.g. _epsilon). The buyer cannot set _epsilon.
	MaxOutputBytes int64
	MaxOutputFiles int
	MaxRuntimeSecs int
}

// RunResult is the runner's product. Output is the single output object's bytes
// (P1 ships one output object; multi-file is a later phase). Logs are the raw
// algorithm logs — the worker NEVER returns these to the buyer directly; they
// pass a log gate first (design §7.4).
type RunResult struct {
	OutputKind string
	Output     []byte
	Logs       []byte
	// Attestation is the L2 TEE remote-attestation report (raw JSON), set only
	// by a confidential (TEE) runner (design P3); nil for L1 runners.
	Attestation []byte
}

// Runner executes an algorithm against a dataset in isolation and returns ONLY
// the output. Implementations progress by isolation strength (design §7.2):
//
//	mockRunner   — deterministic, in-process; for tests and no-docker dev
//	dockerRunner — `docker run --network none` (lands with the algorithm image)
//	gvisor/firecracker/tee — later phases
type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
	Kind() string
	// NeedsStagedData reports whether the worker must stage the dataset bytes to
	// a local path (RunRequest.DataPath) before calling Run — true for runners
	// that mount real data (docker/gVisor/TEE), false for the in-process mock.
	NeedsStagedData() bool
}

// MockRunner is a deterministic in-process runner for tests and docker-less
// dev. It does NOT read the dataset; it synthesizes a small output that depends
// only on the algorithm/output kind, so the full submit→run→release loop can be
// exercised without a container runtime.
//
// Test hooks: a job param "_mock_output_bytes" (number) makes it emit that many
// bytes (exercises the output-size gate); "_mock_output_raw" (string) makes it
// emit those exact bytes verbatim (exercises the structural output gate with a
// non-JSON / unstructured payload, e.g. a malicious algorithm dumping raw data).
type MockRunner struct{}

// NewMockRunner returns a MockRunner.
func NewMockRunner() Runner { return MockRunner{} }

// Kind identifies the runner in audit/metrics.
func (MockRunner) Kind() string { return "mock" }

// NeedsStagedData is false: the mock ignores the dataset.
func (MockRunner) NeedsStagedData() bool { return false }

// Run synthesizes a deterministic output for the algorithm's output kind.
func (MockRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	// Optional oversize hook for the size gate test.
	if n, ok := numParam(req.Job.Params, "_mock_output_bytes"); ok && n > 0 {
		blob := make([]byte, n)
		for i := range blob {
			blob[i] = 'A'
		}
		return RunResult{OutputKind: req.Algorithm.OutputKind, Output: blob}, nil
	}
	// Optional raw (non-JSON, unstructured) hook for the structural gate test:
	// emit the given bytes verbatim, simulating a malicious algorithm that returns
	// a raw blob instead of a structured aggregate.
	if raw, ok := req.Job.Params["_mock_output_raw"].(string); ok && raw != "" {
		return RunResult{OutputKind: req.Algorithm.OutputKind, Output: []byte(raw)}, nil
	}

	// Federated sub-job (P4-a): emit deterministic-but-dataset-varying local
	// params (fedparams-v1) so FedAvg has real numbers to average. The real
	// training image is P4-b; the schema stays the same.
	if req.Algorithm.Runtime == RuntimeFedLogreg {
		seed := 0
		for _, c := range req.Job.DatasetID {
			seed += int(c)
		}
		w0 := float64(seed%7) + 1
		params := map[string]any{
			"_format":   "fedparams-v1",
			"weights":   []float64{w0, w0 / 2, 1},
			"intercept": float64(seed % 3),
			"n":         10 + seed%5,
		}
		b, _ := json.Marshal(params)
		return RunResult{OutputKind: OutputModel, Output: b, Logs: []byte("mock: fed-logreg local params")}, nil
	}

	// PSI sub-job (Direction D 阶段1): emit this party's set (psi-set-v1). Every
	// dataset shares the common elements (so an intersection is non-trivial) plus
	// one dataset-specific element. The real privacy-preserving extractor is 阶段2.
	if req.Algorithm.Runtime == RuntimePSIExtract {
		only := req.Job.DatasetID
		if len(only) > 8 {
			only = only[:8]
		}
		set := map[string]any{
			"_format":  "psi-set-v1",
			"elements": []string{"shared-a", "shared-b", "only-" + only},
		}
		b, _ := json.Marshal(set)
		return RunResult{OutputKind: OutputAggregate, Output: b, Logs: []byte("mock: psi-extract party set")}, nil
	}

	kind := req.Algorithm.OutputKind
	switch kind {
	case OutputModel:
		// A tiny opaque "model" plus its metrics — the platform never
		// deserializes buyer output (design §7.4), so bytes are just bytes.
		payload := map[string]any{
			"_format":    "mock-model-v1",
			"algorithm":  req.Algorithm.Name,
			"version":    req.Algorithm.Version,
			"trained":    true,
			"metrics":    map[string]any{"accuracy": 0.0, "note": "mock runner — no real training"},
			"dataset_id": req.Job.DatasetID,
		}
		b, _ := json.Marshal(payload)
		return RunResult{OutputKind: kind, Output: b, Logs: []byte("mock: training complete")}, nil
	default: // metrics / aggregate / table
		payload := map[string]any{
			"_format":     "mock-metrics-v1",
			"algorithm":   req.Algorithm.Name,
			"output_kind": kind,
			"result":      map[string]any{"rows": 0, "note": "mock runner"},
		}
		b, _ := json.Marshal(payload)
		return RunResult{OutputKind: kind, Output: b, Logs: []byte("mock: computed")}, nil
	}
}

// numParam extracts a numeric param (JSON numbers decode as float64).
func numParam(params map[string]any, key string) (int, bool) {
	if params == nil {
		return 0, false
	}
	switch v := params[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}

// outputObjectKey is the canonical storage key for a job's output object. The
// algorithm never names the output file (anti-exfil; design §7.4) — the
// platform does.
func outputObjectKey(jobID string) string {
	return fmt.Sprintf("compute-outputs/%s/output.bin", jobID)
}
