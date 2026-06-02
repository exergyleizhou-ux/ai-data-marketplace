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
	DataKey        string // dataset object-storage key (read-only input)
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
}

// MockRunner is a deterministic in-process runner for tests and docker-less
// dev. It does NOT read the dataset; it synthesizes a small output that depends
// only on the algorithm/output kind, so the full submit→run→release loop can be
// exercised without a container runtime.
//
// Test hook: a job param "_mock_output_bytes" (number) makes it emit that many
// bytes, used to exercise the output-size gate.
type MockRunner struct{}

// NewMockRunner returns a MockRunner.
func NewMockRunner() Runner { return MockRunner{} }

// Kind identifies the runner in audit/metrics.
func (MockRunner) Kind() string { return "mock" }

// Run synthesizes a deterministic output for the algorithm's output kind.
func (MockRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	// Optional oversize hook for the gate test.
	if n, ok := numParam(req.Job.Params, "_mock_output_bytes"); ok && n > 0 {
		blob := make([]byte, n)
		for i := range blob {
			blob[i] = 'A'
		}
		return RunResult{OutputKind: req.Algorithm.OutputKind, Output: blob}, nil
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
