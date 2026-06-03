package compute

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
)

// defaultDPClip bounds each party's per-coordinate contribution before averaging.
// Overridable per job via fed.Params["dp_clip"].
const defaultDPClip = 10.0

// laplaceNoise draws a sample from Laplace(0, b) using crypto/rand. DP noise must
// be fresh (no fixed seed) — non-reproducible, matching the dp_stats stance.
func laplaceNoise(b float64) float64 {
	if b <= 0 {
		return 0
	}
	// Uniform u in (-0.5, 0.5], then inverse-CDF: x = -b·sgn(u)·ln(1-2|u|).
	const denom = 1 << 53
	n, err := rand.Int(rand.Reader, big.NewInt(denom))
	if err != nil {
		return 0 // fail closed: no noise rather than panic (caller still has clipped mean)
	}
	u := float64(n.Int64())/float64(denom) - 0.5
	if u == 0 {
		u = 1e-12
	}
	sgn := 1.0
	if u < 0 {
		sgn = -1.0
	}
	return -b * sgn * math.Log(1-2*math.Abs(u))
}

// dpFedAvg computes a central differentially-private federated average:
// clip each party per-coordinate to [-clip, clip], take the sample-weighted mean,
// then add Laplace(0, Δ/ε) noise per coordinate, where Δ = 2·clip·maxFrac and
// maxFrac = max(n_k)/Σn_k is the largest single-party weight fraction (the L1
// sensitivity of the bounded weighted mean).
//
// HONESTY: this is CENTRAL DP on the released aggregate — the platform clips and
// noises the mean. It is NOT local DP / DP-SGD (per-example clipping at training
// time); that is future work. noise is injected for deterministic testing; the
// production caller passes laplaceNoise.
func dpFedAvg(partials []Partial, epsilon, clip float64, noise func(b float64) float64) ([]byte, error) {
	if len(partials) == 0 {
		return nil, ErrNoPartials
	}
	if epsilon <= 0 {
		return nil, fmt.Errorf("compute: dp epsilon must be > 0")
	}
	if clip <= 0 {
		clip = defaultDPClip
	}
	dim := len(partials[0].Weights)
	totalN := 0
	maxN := 0
	for _, p := range partials {
		if len(p.Weights) != dim {
			return nil, ErrDimMismatch
		}
		totalN += p.N
		if p.N > maxN {
			maxN = p.N
		}
	}
	if totalN <= 0 {
		return nil, ErrZeroSamples
	}
	clamp := func(x float64) float64 { return math.Max(-clip, math.Min(clip, x)) }

	w := make([]float64, dim)
	var intercept float64
	for _, p := range partials {
		f := float64(p.N) / float64(totalN)
		for i := range w {
			w[i] += f * clamp(p.Weights[i])
		}
		intercept += f * clamp(p.Intercept)
	}

	// Sensitivity of the clipped weighted mean per coordinate, then noise scale.
	maxFrac := float64(maxN) / float64(totalN)
	sensitivity := 2 * clip * maxFrac
	b := sensitivity / epsilon
	for i := range w {
		w[i] += noise(b)
	}
	intercept += noise(b)

	out := map[string]any{
		"_format":      "fedmodel-v1",
		"weights":      w,
		"intercept":    intercept,
		"n_total":      totalN,
		"participants": len(partials),
		"dp": map[string]any{
			"mechanism": "laplace-central",
			"epsilon":   epsilon,
			"clip":      clip,
			"note":      "central DP on the released aggregate; not local DP/DP-SGD",
		},
	}
	bts, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("compute: marshal dp joint model: %w", err)
	}
	return bts, nil
}
