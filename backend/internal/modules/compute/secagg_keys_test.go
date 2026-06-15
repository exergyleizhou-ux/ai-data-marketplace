package compute

import (
	"math"
	"testing"
)

// sumPartials returns the element-wise sum of the parties' weighted
// contributions (weights and intercept) — what a plaintext aggregator would see.
func sumPartials(t *testing.T, parts []Partial) ([]float64, float64) {
	t.Helper()
	dim := len(parts[0].Weights)
	w := make([]float64, dim)
	var ic float64
	for _, p := range parts {
		for i := range w {
			w[i] += p.Weights[i]
		}
		ic += p.Intercept
	}
	return w, ic
}

// TestPairwiseMasksCancelToTrueSum is the secure-aggregation property that the
// existing tests ASSUME but never produce: real pairwise masks, derived from
// ECDH shared secrets, that cancel across all parties. Each party masks its own
// weighted contribution using only its private key + peers' public keys; the
// platform never distributes the masks. The sum of masked contributions must
// equal the sum of the true contributions, yet no single masked contribution may
// equal its true value (the aggregator never sees a party's real params).
func TestPairwiseMasksCancelToTrueSum(t *testing.T) {
	// Four parties' true weighted contributions (n_k·x_k) over a 3-dim model.
	truth := []Partial{
		{Weights: []float64{1.5, -2.0, 3.0}, Intercept: 0.5, N: 1},
		{Weights: []float64{4.0, 4.0, -1.0}, Intercept: 2.0, N: 3},
		{Weights: []float64{-2.5, 1.0, 0.0}, Intercept: -1.0, N: 2},
		{Weights: []float64{0.0, 5.0, 2.0}, Intercept: 4.0, N: 4},
	}

	// Each party generates an ECDH keypair and publishes only its public key.
	parties := make([]*secAggParty, len(truth))
	pubs := map[int][]byte{}
	for i := range truth {
		p, err := newSecAggParty(i)
		if err != nil {
			t.Fatalf("newSecAggParty(%d): %v", i, err)
		}
		parties[i] = p
		pubs[i] = p.pubKey()
	}

	// Each party masks its contribution against every peer's public key.
	masked := make([]Partial, len(truth))
	for i, p := range parties {
		peers := map[int][]byte{}
		for j := range pubs {
			if j != i {
				peers[j] = pubs[j]
			}
		}
		m, err := p.mask(truth[i], peers)
		if err != nil {
			t.Fatalf("party %d mask: %v", i, err)
		}
		masked[i] = m
		// The masked contribution must differ from the true one in every party.
		if m.Weights[0] == truth[i].Weights[0] && m.Intercept == truth[i].Intercept {
			t.Fatalf("party %d: masked contribution equals the true one — no real mask applied", i)
		}
		// Clear sample count is NOT masked (the aggregator needs it to weight).
		if m.N != truth[i].N {
			t.Fatalf("party %d: N must stay clear, got %d want %d", i, m.N, truth[i].N)
		}
	}

	// The masks must cancel: Σ masked == Σ true.
	wantW, wantI := sumPartials(t, truth)
	gotW, gotI := sumPartials(t, masked)
	for i := range wantW {
		if math.Abs(gotW[i]-wantW[i]) > 1e-6 {
			t.Fatalf("masked sum weights[%d]=%v want %v (masks did not cancel)", i, gotW[i], wantW[i])
		}
	}
	if math.Abs(gotI-wantI) > 1e-6 {
		t.Fatalf("masked sum intercept=%v want %v (masks did not cancel)", gotI, wantI)
	}
}

// TestSecAggRoundTripEqualsFedAvg proves the end-to-end guarantee: masking each
// party's contribution and running MaskedSumAggregator yields the IDENTICAL joint
// model as plaintext FedAvg over the same contributions — but the aggregator only
// ever sees masked values.
func TestSecAggRoundTripEqualsFedAvg(t *testing.T) {
	// True per-sample params x_k and counts n_k; weighted contribution = n_k·x_k.
	type party struct {
		x []float64
		i float64
		n int
	}
	raw := []party{
		{x: []float64{2, 4}, i: 1, n: 1},
		{x: []float64{4, 8}, i: 5, n: 3},
	}
	truth := make([]Partial, len(raw))
	for k, r := range raw {
		w := make([]float64, len(r.x))
		for i := range w {
			w[i] = float64(r.n) * r.x[i]
		}
		truth[k] = Partial{Weights: w, Intercept: float64(r.n) * r.i, N: r.n}
	}

	parties := make([]*secAggParty, len(truth))
	pubs := map[int][]byte{}
	for i := range truth {
		p, _ := newSecAggParty(i)
		parties[i] = p
		pubs[i] = p.pubKey()
	}
	masked := make([]Partial, len(truth))
	for i, p := range parties {
		peers := map[int][]byte{}
		for j := range pubs {
			if j != i {
				peers[j] = pubs[j]
			}
		}
		masked[i], _ = p.mask(truth[i], peers)
	}

	out, err := MaskedSumAggregator{}.Aggregate(masked)
	if err != nil {
		t.Fatalf("aggregate masked: %v", err)
	}
	m := decodeJoint(t, out)
	// Plaintext FedAvg target: [3.5, 7], intercept 4.
	want := []float64{3.5, 7}
	for i := range want {
		if math.Abs(m.Weights[i]-want[i]) > 1e-6 {
			t.Fatalf("weights[%d]=%v want %v", i, m.Weights[i], want[i])
		}
	}
	if math.Abs(m.Intercept-4) > 1e-6 {
		t.Fatalf("intercept=%v want 4", m.Intercept)
	}
}
