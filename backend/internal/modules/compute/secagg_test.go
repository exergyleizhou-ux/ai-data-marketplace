package compute

import (
	"encoding/json"
	"math"
	"testing"
)

// decodeJoint unmarshals a fedmodel-v1 joint model for assertions.
func decodeJoint(t *testing.T, out []byte) struct {
	Weights      []float64 `json:"weights"`
	Intercept    float64   `json:"intercept"`
	NTotal       int       `json:"n_total"`
	Participants int       `json:"participants"`
	Aggregation  string    `json:"aggregation"`
} {
	t.Helper()
	var m struct {
		Weights      []float64 `json:"weights"`
		Intercept    float64   `json:"intercept"`
		NTotal       int       `json:"n_total"`
		Participants int       `json:"participants"`
		Aggregation  string    `json:"aggregation"`
	}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal joint model: %v", err)
	}
	return m
}

// TestMaskedSumCancelsToFedAvg is the core secure-aggregation property: each
// party submits its WEIGHTED contribution (n_k·x_k) plus a pairwise mask that
// sums to zero across parties. The aggregator sees only masked values, never a
// single party's params — yet the result equals plaintext weighted FedAvg.
//
// Plaintext target (same as TestFedAvgWeightedMean): x_A=[2,4] n=1, x_B=[4,8] n=3
// → FedAvg weights [3.5,7], intercept 4.
func TestMaskedSumCancelsToFedAvg(t *testing.T) {
	// Pairwise mask M added by A and subtracted by B (Σ mask = 0).
	maskW := []float64{100, -50}
	maskI := 7.0
	// A submits n_A·x_A + M = [2,4]+[100,-50] ; intercept 1*1 + 7
	a := Partial{Weights: []float64{2 + maskW[0], 4 + maskW[1]}, Intercept: 1 + maskI, N: 1}
	// B submits n_B·x_B − M = [12,24]-[100,-50] ; intercept 5*3 − 7
	b := Partial{Weights: []float64{12 - maskW[0], 24 - maskW[1]}, Intercept: 15 - maskI, N: 3}

	// Sanity: the masked inputs do NOT equal the true weighted contributions
	// (the aggregator genuinely never sees a party's real params).
	if a.Weights[0] == 2 || b.Weights[0] == 12 {
		t.Fatal("masked inputs must differ from the true weighted contributions")
	}

	out, err := MaskedSumAggregator{}.Aggregate([]Partial{a, b})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	m := decodeJoint(t, out)
	want := []float64{3.5, 7}
	for i := range want {
		if math.Abs(m.Weights[i]-want[i]) > 1e-9 {
			t.Fatalf("weights[%d]=%v want %v", i, m.Weights[i], want[i])
		}
	}
	if math.Abs(m.Intercept-4) > 1e-9 {
		t.Fatalf("intercept=%v want 4", m.Intercept)
	}
	if m.NTotal != 4 || m.Participants != 2 {
		t.Fatalf("n_total=%d participants=%d", m.NTotal, m.Participants)
	}
	if m.Aggregation != "masked-sum" {
		t.Fatalf("aggregation marker = %q, want masked-sum", m.Aggregation)
	}
}

// TestMaskedSumThreePartiesMasksCancel exercises >2 parties with a cyclic set of
// pairwise masks that sum to zero.
func TestMaskedSumThreePartiesMasksCancel(t *testing.T) {
	// True weighted contributions: A n=1 x=[1], B n=1 x=[2], C n=2 x=[3]
	// Σ n·x = 1 + 2 + 6 = 9 ; Σ n = 4 → FedAvg = 2.25
	// Cyclic masks: A+=m1, B+=m2, C+=-(m1+m2) → Σ = 0
	m1, m2 := 10.0, -7.0
	parts := []Partial{
		{Weights: []float64{1 + m1}, Intercept: 0, N: 1},
		{Weights: []float64{2 + m2}, Intercept: 0, N: 1},
		{Weights: []float64{6 - (m1 + m2)}, Intercept: 0, N: 2},
	}
	out, err := MaskedSumAggregator{}.Aggregate(parts)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	m := decodeJoint(t, out)
	if math.Abs(m.Weights[0]-2.25) > 1e-9 {
		t.Fatalf("weights[0]=%v want 2.25", m.Weights[0])
	}
	if m.NTotal != 4 || m.Participants != 3 {
		t.Fatalf("n_total=%d participants=%d", m.NTotal, m.Participants)
	}
}

func TestMaskedSumRejectsEmpty(t *testing.T) {
	if _, err := (MaskedSumAggregator{}).Aggregate(nil); err != ErrNoPartials {
		t.Fatalf("empty partials must return ErrNoPartials, got %v", err)
	}
}

func TestMaskedSumRejectsDimMismatch(t *testing.T) {
	parts := []Partial{
		{Weights: []float64{1, 2}, N: 1},
		{Weights: []float64{1}, N: 1},
	}
	if _, err := (MaskedSumAggregator{}).Aggregate(parts); err != ErrDimMismatch {
		t.Fatalf("dimension mismatch must return ErrDimMismatch, got %v", err)
	}
}

func TestMaskedSumRejectsZeroSamples(t *testing.T) {
	parts := []Partial{
		{Weights: []float64{1}, N: 0},
		{Weights: []float64{2}, N: 0},
	}
	if _, err := (MaskedSumAggregator{}).Aggregate(parts); err != ErrZeroSamples {
		t.Fatalf("zero total samples must return ErrZeroSamples, got %v", err)
	}
}

func TestMaskedSumKind(t *testing.T) {
	if (MaskedSumAggregator{}).Kind() != "masked-sum" {
		t.Fatalf("kind = %q", (MaskedSumAggregator{}).Kind())
	}
}
