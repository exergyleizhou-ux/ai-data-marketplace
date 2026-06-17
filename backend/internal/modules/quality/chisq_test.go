package quality

import (
	"math"
	"testing"
)

func TestChiSquarePValueKnownCriticalValues(t *testing.T) {
	// Critical chi-square values at p=0.05 from standard tables.
	cases := []struct {
		stat float64
		df   int
	}{
		{3.841, 1},
		{5.991, 2},
		{15.507, 8},
		{16.919, 9},
	}
	for _, c := range cases {
		p := chiSquareP(c.stat, c.df)
		if math.Abs(p-0.05) > 0.005 {
			t.Errorf("chiSquareP(%.3f, %d) = %.4f, want ~0.05", c.stat, c.df, p)
		}
	}
}

func TestChiSquarePValueEdges(t *testing.T) {
	if p := chiSquareP(0, 5); p != 1 {
		t.Errorf("chiSquareP(0,5) = %v, want 1", p)
	}
	if p := chiSquareP(1000, 8); p > 1e-6 {
		t.Errorf("chiSquareP(1000,8) = %v, want ~0", p)
	}
	// Monotonic: larger statistic => smaller tail probability.
	if chiSquareP(20, 8) >= chiSquareP(5, 8) {
		t.Error("p-value must decrease as the statistic grows")
	}
}

func TestBenjaminiHochbergOrderPreservedAndMonotone(t *testing.T) {
	in := []float64{0.5, 0.001}
	adj := benjaminiHochberg(in)
	if len(adj) != 2 {
		t.Fatalf("len=%d", len(adj))
	}
	if math.Abs(adj[0]-0.5) > 1e-9 {
		t.Errorf("adj[0]=%v, want 0.5 (position preserved)", adj[0])
	}
	if math.Abs(adj[1]-0.002) > 1e-9 {
		t.Errorf("adj[1]=%v, want 0.002", adj[1])
	}

	// All-equal-step input collapses to the largest adjusted value.
	all := benjaminiHochberg([]float64{0.01, 0.02, 0.03, 0.04, 0.05})
	for i, v := range all {
		if math.Abs(v-0.05) > 1e-9 {
			t.Errorf("adj[%d]=%v, want 0.05", i, v)
		}
	}
}
