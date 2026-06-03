package compute

import (
	"encoding/json"
	"math"
	"testing"
)

// zeroNoise injects no noise → dpFedAvg should equal the clipped weighted mean.
func zeroNoise(float64) float64 { return 0 }

func TestDPFedAvgNoNoiseEqualsClippedMean(t *testing.T) {
	// Two parties within the clip bound → clipping is a no-op; equals plain FedAvg.
	parts := []Partial{
		{Weights: []float64{2, 4}, Intercept: 1, N: 1},
		{Weights: []float64{4, 8}, Intercept: 5, N: 3},
	}
	out, err := dpFedAvg(parts, 1.0, 100.0, zeroNoise)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Weights   []float64      `json:"weights"`
		Intercept float64        `json:"intercept"`
		DP        map[string]any `json:"dp"`
	}
	_ = json.Unmarshal(out, &m)
	want := []float64{3.5, 7} // (2·1+4·3)/4, (4·1+8·3)/4
	for i := range want {
		if math.Abs(m.Weights[i]-want[i]) > 1e-9 {
			t.Fatalf("w[%d]=%v want %v", i, m.Weights[i], want[i])
		}
	}
	if math.Abs(m.Intercept-4) > 1e-9 {
		t.Fatalf("intercept=%v want 4", m.Intercept)
	}
	if m.DP == nil || m.DP["mechanism"] != "laplace-central" {
		t.Fatalf("missing dp metadata: %+v", m.DP)
	}
}

func TestDPFedAvgClips(t *testing.T) {
	// One party's weight (100) exceeds clip=5 → clipped to 5 before averaging.
	parts := []Partial{
		{Weights: []float64{100}, Intercept: 0, N: 1},
		{Weights: []float64{1}, Intercept: 0, N: 1},
	}
	out, _ := dpFedAvg(parts, 1.0, 5.0, zeroNoise)
	var m struct {
		Weights []float64 `json:"weights"`
	}
	_ = json.Unmarshal(out, &m)
	want := (5.0 + 1.0) / 2 // clipped(100)=5, clipped(1)=1, equal weights
	if math.Abs(m.Weights[0]-want) > 1e-9 {
		t.Fatalf("clipped mean=%v want %v", m.Weights[0], want)
	}
}

func TestDPFedAvgScale(t *testing.T) {
	// noiseFn returns its own scale b → output weight = mean + b, where
	// b = Δ/ε, Δ = 2·clip·maxFrac. Equal N (2 parties) → maxFrac=0.5.
	parts := []Partial{
		{Weights: []float64{2}, Intercept: 0, N: 1},
		{Weights: []float64{4}, Intercept: 0, N: 1},
	}
	clip, eps := 10.0, 2.0
	identity := func(b float64) float64 { return b } // reveal the scale
	out, _ := dpFedAvg(parts, eps, clip, identity)
	var m struct {
		Weights []float64 `json:"weights"`
	}
	_ = json.Unmarshal(out, &m)
	mean := 3.0                     // (2+4)/2
	wantB := (2 * clip * 0.5) / eps // Δ/ε = (2·10·0.5)/2 = 5
	if math.Abs(m.Weights[0]-(mean+wantB)) > 1e-9 {
		t.Fatalf("scale: got %v want %v (mean %v + b %v)", m.Weights[0], mean+wantB, mean, wantB)
	}
}

func TestDPFedAvgErrors(t *testing.T) {
	parts := []Partial{{Weights: []float64{1}, N: 1}}
	if _, err := dpFedAvg(parts, 0, 10, zeroNoise); err == nil {
		t.Fatal("want error for epsilon<=0")
	}
	if _, err := dpFedAvg(nil, 1, 10, zeroNoise); err == nil {
		t.Fatal("want error for empty partials")
	}
}

// laplaceNoise must produce spread (not constant) and average near zero.
func TestLaplaceNoiseLive(t *testing.T) {
	var sum, sumAbs float64
	const n = 4000
	b := 3.0
	distinct := map[float64]bool{}
	for i := 0; i < n; i++ {
		x := laplaceNoise(b)
		sum += x
		sumAbs += math.Abs(x)
		distinct[x] = true
	}
	if len(distinct) < n/2 {
		t.Fatalf("noise not random enough: %d distinct of %d", len(distinct), n)
	}
	if math.Abs(sum/n) > 0.5 { // mean ~ 0
		t.Fatalf("noise mean far from 0: %v", sum/n)
	}
	if mad := sumAbs / n; mad < 1.0 || mad > 6.0 { // E|Laplace(b)| = b = 3
		t.Fatalf("noise mean-abs-dev=%v, expected near %v", mad, b)
	}
}
