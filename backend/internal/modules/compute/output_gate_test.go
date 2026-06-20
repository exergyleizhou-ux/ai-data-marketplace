package compute

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// zipOf builds an in-memory zip with the given name->content entries (used to
// mimic the real algorithm contract: output.bin = zip{model.json, metrics.json}).
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestGateOutput_RealPayloadsPass pins that every output shape the LIVE runners
// actually emit passes the gate — the mock JSON, the federated/PSI sub-job JSON,
// and the real algorithm zip-of-json. A regression here would break production.
func TestGateOutput_RealPayloadsPass(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		output []byte
	}{
		{"mock-model-v1", OutputModel, mustJSON(t, map[string]any{
			"_format": "mock-model-v1", "algorithm": "logreg", "version": 1, "trained": true,
			"metrics": map[string]any{"accuracy": 0.0, "note": "mock"}, "dataset_id": "ds_123",
		})},
		{"mock-metrics-v1", OutputMetrics, mustJSON(t, map[string]any{
			"_format": "mock-metrics-v1", "algorithm": "dp_stats", "output_kind": "metrics",
			"result": map[string]any{"rows": 0, "note": "mock runner"},
		})},
		{"fedparams-v1", OutputModel, mustJSON(t, map[string]any{
			"_format": "fedparams-v1", "weights": []float64{3, 1.5, 1}, "intercept": 1.0, "n": 12,
		})},
		{"psi-set-v1", OutputAggregate, mustJSON(t, map[string]any{
			"_format": "psi-set-v1", "elements": []string{"shared-a", "shared-b", "only-abcd1234"},
		})},
		{"real-algo-zip", OutputModel, zipOf(t, map[string]string{
			"model.json":   `{"nde":2.82,"nie":8.19,"ate":11.0,"coefficients":[0.1,0.2,0.3]}`,
			"metrics.json": `{"r2":0.99,"n":840,"prop_mediated":0.74}`,
		})},
		{"single-metrics-zip", OutputMetrics, zipOf(t, map[string]string{
			"metrics.json": `{"mean":3.14,"count":840,"min_p":0.001}`,
		})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if v := GateOutput(c.kind, c.output, policyForKind(c.kind, 0)); v != nil {
				t.Fatalf("expected pass, got violation %s: %s", v.Reason, v.Detail)
			}
		})
	}
}

// TestGateOutput_Rejects covers the anti-exfil teeth: an algorithm author can
// only return a structured aggregate within bounded information content.
func TestGateOutput_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		kind       string
		output     []byte
		wantReason string
	}{
		{
			name:       "raw binary blob (dataset dumped as output.bin)",
			kind:       OutputAggregate,
			output:     []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 'r', 'a', 'w'},
			wantReason: ReasonNotStructured,
		},
		{
			name:       "zip smuggling a csv entry",
			kind:       OutputModel,
			output:     zipOf(t, map[string]string{"model.json": `{"a":1}`, "data.csv": "id,ssn\n1,123-45-6789\n"}),
			wantReason: ReasonNotStructured,
		},
		{
			name:       "zip with malformed json entry",
			kind:       OutputModel,
			output:     zipOf(t, map[string]string{"model.json": `{not valid json`}),
			wantReason: ReasonNotStructured,
		},
		{
			name:       "top-level json array (not an object)",
			kind:       OutputAggregate,
			output:     []byte(`[1,2,3]`),
			wantReason: ReasonNotStructured,
		},
		{
			name:       "huge base64 string exfil in a field",
			kind:       OutputAggregate,
			output:     mustJSON(t, map[string]any{"summary": strings.Repeat("QUJDREVGR0g", 2000)}), // ~22KB > 8KiB aggregate string budget
			wantReason: ReasonStringsTooLarge,
		},
		{
			name:       "flattened dataset as a numeric array",
			kind:       OutputAggregate,
			output:     mustJSON(t, map[string]any{"data": makeFloats(20000)}), // > 10k aggregate numeric budget
			wantReason: ReasonTooManyNumbers,
		},
		{
			name:       "high-entropy compressed blob string (under byte budget, model kind)",
			kind:       OutputModel,
			output:     mustJSON(t, map[string]any{"blob": strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 40)}), // 2560 chars, uniform over 64 syms = 6 bits/byte
			wantReason: ReasonHighEntropyString,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := GateOutput(c.kind, c.output, policyForKind(c.kind, 0))
			if v == nil {
				t.Fatalf("expected violation %s, got pass", c.wantReason)
			}
			if v.Reason != c.wantReason {
				t.Fatalf("expected reason %s, got %s (%s)", c.wantReason, v.Reason, v.Detail)
			}
		})
	}
}

// TestGateOutput_ByteCap isolates the outer size cap via an explicit policy.
func TestGateOutput_ByteCap(t *testing.T) {
	p := policyForKind(OutputAggregate, 16) // offer override: 16-byte cap
	out := []byte(`{"x":123456789012345}`)  // valid JSON, > 16 bytes
	v := GateOutput(OutputAggregate, out, p)
	if v == nil || v.Reason != ReasonTooLarge {
		t.Fatalf("expected %s, got %v", ReasonTooLarge, v)
	}
}

// TestGateOutput_LongLowEntropyStringPasses guards against an entropy
// false-positive: a long but repetitive string (low entropy) is fine.
func TestGateOutput_LongLowEntropyStringPasses(t *testing.T) {
	out := mustJSON(t, map[string]any{"note": strings.Repeat("a", 1000)})
	if v := GateOutput(OutputModel, out, policyForKind(OutputModel, 0)); v != nil {
		t.Fatalf("expected pass, got %s: %s", v.Reason, v.Detail)
	}
}

func makeFloats(n int) []float64 {
	f := make([]float64, n)
	for i := range f {
		f[i] = float64(i)
	}
	return f
}
