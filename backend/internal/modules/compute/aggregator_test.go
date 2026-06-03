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
	var m struct {
		Weights   []float64 `json:"weights"`
		Intercept float64   `json:"intercept"`
	}
	_ = json.Unmarshal(out, &m)
	if m.Weights[0] != 1 || m.Weights[2] != 3 || m.Intercept != 9 {
		t.Fatalf("identity failed: %+v", m)
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
	if _, err := parsePartial([]byte(`{"_format":"mock-model-v1"}`)); err == nil {
		t.Fatal("want error on wrong format")
	}
}
