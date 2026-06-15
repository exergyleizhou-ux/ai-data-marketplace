package compute

import (
	"math"
	"testing"
)

// These tests pin the security INVARIANT of the secure-aggregation masking, so a
// future change that weakens it (e.g. trivial/zero masks, or masking that leaks a
// subset sum) fails CI rather than silently becoming privacy theater.
//
// THREAT MODEL (honest): pure pairwise masking (secagg.go) protects against an
// honest-but-curious AGGREGATOR acting alone — it sees only masked contributions
// and cannot recover any single party's params or any proper-subset sum. It does
// NOT by itself withstand the aggregator COLLUDING with N−1 parties (that needs
// per-party masks + Shamir recovery, 阶段3). We therefore assert only what holds:
// the aggregator alone learns nothing about individuals or sub-coalitions.

// maskedCohort builds N parties, masks each party's contribution against all
// peers, and returns the masked and the true contributions.
func maskedCohort(t *testing.T, truth []Partial) (masked, plain []Partial) {
	t.Helper()
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
	masked = make([]Partial, len(truth))
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
	}
	return masked, truth
}

// TestAggregatorCannotRecoverProperSubsetSum is the core privacy invariant: the
// aggregator may sum any subset of the masked contributions it received, but for
// every PROPER non-empty subset the result is off by the uncancelled cross-masks
// — so it can neither isolate an individual (singleton) nor any sub-coalition.
// Only the FULL cohort cancels. If masking were trivial, the masked subset sum
// would equal the true subset sum and this test would fail.
func TestAggregatorCannotRecoverProperSubsetSum(t *testing.T) {
	truth := []Partial{
		{Weights: []float64{1.5, -2.0}, Intercept: 0.5, N: 1},
		{Weights: []float64{4.0, 4.0}, Intercept: 2.0, N: 3},
		{Weights: []float64{-2.5, 1.0}, Intercept: -1.0, N: 2},
		{Weights: []float64{0.0, 5.0}, Intercept: 4.0, N: 4},
	}
	masked, plain := maskedCohort(t, truth)
	n := len(truth)
	dim := len(truth[0].Weights)

	subsetSum := func(parts []Partial, mask uint) ([]float64, float64) {
		w := make([]float64, dim)
		var ic float64
		for i := 0; i < n; i++ {
			if mask&(1<<uint(i)) != 0 {
				for d := 0; d < dim; d++ {
					w[d] += parts[i].Weights[d]
				}
				ic += parts[i].Intercept
			}
		}
		return w, ic
	}

	full := uint((1 << uint(n)) - 1)
	for mask := uint(1); mask < full; mask++ { // every proper non-empty subset
		mw, mi := subsetSum(masked, mask)
		tw, ti := subsetSum(plain, mask)
		// At least one coordinate must be off by a mask-scale margin (params are
		// O(1–10); masks are O(2^20)), proving the subset sum is not recoverable.
		diff := math.Abs(mi - ti)
		for d := 0; d < dim; d++ {
			diff = math.Max(diff, math.Abs(mw[d]-tw[d]))
		}
		if diff < 1.0 {
			t.Fatalf("proper subset %04b masked sum is recoverable (diff=%v) — masks leaked", mask, diff)
		}
	}

	// Control: the FULL cohort DOES cancel (sanity that the construction works).
	mw, mi := subsetSum(masked, full)
	tw, ti := subsetSum(plain, full)
	for d := 0; d < dim; d++ {
		if math.Abs(mw[d]-tw[d]) > 1e-6 {
			t.Fatalf("full cohort must cancel: weights[%d] off by %v", d, math.Abs(mw[d]-tw[d]))
		}
	}
	if math.Abs(mi-ti) > 1e-6 {
		t.Fatalf("full cohort must cancel: intercept off by %v", math.Abs(mi-ti))
	}
}

// TestSingleMaskedContributionHidesParams asserts a lone masked contribution is
// displaced from its true value by mask scale — an aggregator inspecting one
// party's submission learns nothing useful about its params.
func TestSingleMaskedContributionHidesParams(t *testing.T) {
	truth := []Partial{
		{Weights: []float64{1.0, 2.0, 3.0}, Intercept: 0.5, N: 2},
		{Weights: []float64{1.0, 2.0, 3.0}, Intercept: 0.5, N: 2},
	}
	masked, _ := maskedCohort(t, truth)
	displaced := false
	for d := range masked[0].Weights {
		if math.Abs(masked[0].Weights[d]-truth[0].Weights[d]) > 1.0 {
			displaced = true
		}
	}
	if !displaced {
		t.Fatal("masked contribution is too close to the true params — masking is ineffective")
	}
}
