package compute

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParsePSISet(t *testing.T) {
	raw := []byte(`{"_format":"psi-set-v1","elements":["b","a","a"]}`)
	set, err := parsePSISet(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Returned as-is (the orchestrator dedupes); just confirm round-trip.
	if len(set) != 3 || set[0] != "b" {
		t.Fatalf("elements = %v", set)
	}
}

func TestParsePSISet_RejectsWrongFormat(t *testing.T) {
	if _, err := parsePSISet([]byte(`{"_format":"fedparams-v1","weights":[1]}`)); err == nil {
		t.Fatal("a non-psi-set payload must be rejected")
	}
}

func TestMockRunner_EmitsPSISetForPSIExtract(t *testing.T) {
	out, err := MockRunner{}.Run(context.Background(), RunRequest{
		Job:       Job{ID: "j1", DatasetID: "dataset-alpha"},
		Algorithm: Algorithm{Name: "psi-extract", Runtime: RuntimePSIExtract, OutputKind: OutputAggregate},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var p struct {
		Format   string   `json:"_format"`
		Elements []string `json:"elements"`
	}
	if err := json.Unmarshal(out.Output, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Format != "psi-set-v1" {
		t.Fatalf("format = %q, want psi-set-v1", p.Format)
	}
	// The mock emits shared elements (so intersections are non-trivial) plus a
	// dataset-specific one. Parse must accept its own output.
	if _, err := parsePSISet(out.Output); err != nil {
		t.Fatalf("psi-extract output must parse as a psi set: %v", err)
	}
	if len(p.Elements) < 2 {
		t.Fatalf("expected at least the shared elements, got %v", p.Elements)
	}
}

// Two different datasets must share the common elements (so PSI over them yields
// the shared set) but differ in their dataset-specific element.
func TestMockRunner_PSISetsShareCommonAcrossDatasets(t *testing.T) {
	run := func(ds string) []string {
		out, _ := MockRunner{}.Run(context.Background(), RunRequest{
			Job:       Job{ID: "j", DatasetID: ds},
			Algorithm: Algorithm{Runtime: RuntimePSIExtract, OutputKind: OutputAggregate},
		})
		s, _ := parsePSISet(out.Output)
		return s
	}
	a := run("dataset-alpha")
	b := run("dataset-beta")
	res, err := NewMockMPC().RunPSI(context.Background(), [][]string{a, b})
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	if res.Cardinality == 0 {
		t.Fatalf("two psi-extract datasets must share at least the common elements; got empty intersection a=%v b=%v", a, b)
	}
}
