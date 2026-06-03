package compute

import (
	"context"
	"testing"
)

func TestMockRunnerFedParams(t *testing.T) {
	r := NewMockRunner()
	req := RunRequest{
		Algorithm: Algorithm{Name: "fed-logreg", Runtime: RuntimeFedLogreg, OutputKind: OutputModel},
		Job:       Job{DatasetID: "11111111-1111-1111-1111-111111111111"},
	}
	res, err := r.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parsePartial(res.Output)
	if err != nil {
		t.Fatalf("not fedparams-v1: %v", err)
	}
	if len(p.Weights) == 0 || p.N <= 0 {
		t.Fatalf("bad params: %+v", p)
	}
}

// Different datasets must yield different local params so aggregation is meaningful.
func TestMockRunnerFedParamsVaryByDataset(t *testing.T) {
	r := NewMockRunner()
	mk := func(ds string) Partial {
		res, _ := r.Run(context.Background(), RunRequest{
			Algorithm: Algorithm{Runtime: RuntimeFedLogreg, OutputKind: OutputModel},
			Job:       Job{DatasetID: ds},
		})
		p, _ := parsePartial(res.Output)
		return p
	}
	a := mk("aaaaaaaa-0000-0000-0000-000000000000")
	b := mk("bbbbbbbb-1111-1111-1111-111111111111")
	if a.Weights[0] == b.Weights[0] && a.N == b.N {
		t.Fatalf("expected dataset-varying params, got identical: %+v vs %+v", a, b)
	}
}
