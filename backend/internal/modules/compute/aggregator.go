package compute

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Partial is one party's local model contribution to a federated aggregation.
type Partial struct {
	Weights   []float64
	Intercept float64
	N         int // local sample count (FedAvg weighting)
}

// Aggregator combines N parties' local params into one joint model (bytes).
// Implementations:
//   - FedAvgAggregator   — plaintext weighted mean (the platform sees each party's params).
//   - MaskedSumAggregator — secure-aggregation sum: parties pre-mask their weighted
//     contributions with pairwise masks that cancel, so the aggregator sees only
//     masked values, never a single party's params (Direction C 阶段1).
//
// MPCAggregator is reserved for P4-c (secure multi-party computation) and is not
// implemented in the MVP.
type Aggregator interface {
	Aggregate(partials []Partial) ([]byte, error)
	Kind() string
}

var (
	// ErrNoPartials is returned when there is nothing to aggregate.
	ErrNoPartials = errors.New("compute: no partials to aggregate")
	// ErrDimMismatch is returned when parties report different weight dimensions.
	ErrDimMismatch = errors.New("compute: partial weight dimensions differ")
	// ErrZeroSamples is returned when the total sample count is zero.
	ErrZeroSamples = errors.New("compute: total sample count is zero")
)

// FedAvgAggregator implements weighted federated averaging:
//
//	w* = Σ(n_k · w_k) / Σ n_k     (and likewise for the intercept)
//
// This is the real FedAvg math (not a mock). The output is a fedmodel-v1 JSON
// document. The platform only ever sees model parameters, never raw data.
type FedAvgAggregator struct{}

// Kind identifies the aggregator in audit/metrics.
func (FedAvgAggregator) Kind() string { return "fedavg" }

// Aggregate computes the sample-weighted mean of the parties' parameters.
func (FedAvgAggregator) Aggregate(partials []Partial) ([]byte, error) {
	if len(partials) == 0 {
		return nil, ErrNoPartials
	}
	dim := len(partials[0].Weights)
	totalN := 0
	for _, p := range partials {
		if len(p.Weights) != dim {
			return nil, ErrDimMismatch
		}
		totalN += p.N
	}
	if totalN <= 0 {
		return nil, ErrZeroSamples
	}
	w := make([]float64, dim)
	var intercept float64
	for _, p := range partials {
		f := float64(p.N) / float64(totalN)
		for i := range w {
			w[i] += f * p.Weights[i]
		}
		intercept += f * p.Intercept
	}
	out := map[string]any{
		"_format":      "fedmodel-v1",
		"weights":      w,
		"intercept":    intercept,
		"n_total":      totalN,
		"participants": len(partials),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("compute: marshal joint model: %w", err)
	}
	return b, nil
}

// MaskedSumAggregator implements the aggregation half of secure aggregation
// (Bonawitz et al. 2017, Direction C). Each party submits its WEIGHTED local
// contribution (n_k·x_k) plus a PAIRWISE MASK; across all parties the masks sum
// to zero, so:
//
//	Σ_k (n_k·x_k + mask_k) = Σ_k n_k·x_k     (masks cancel)
//	joint = Σ_k n_k·x_k / Σ_k n_k            (= weighted FedAvg)
//
// The aggregator therefore only ever sees masked values y_k, never a single
// party's real params — closing the "platform sees each party's params" gap of
// plaintext FedAvg, while producing the IDENTICAL joint model.
//
// 诚实边界 (honest scope, 阶段1): this is the aggregation math only. It assumes
// the parties already hold pairwise masks that cancel. Generating those masks
// SECURELY inside each sandbox (a key-agreement round) and recovering from
// dropouts are 阶段2/3 — until then this aggregator is a verified building block,
// not yet wired into a user-facing "secure" path (that would be a façade without
// real in-sandbox masking). See docs/superpowers/specs/2026-06-04-direction-c-*.
type MaskedSumAggregator struct{}

// Kind identifies the aggregator in audit/metrics.
func (MaskedSumAggregator) Kind() string { return "masked-sum" }

// Aggregate sums the masked weighted contributions and divides by the total
// sample count. Input contract: each Partial.Weights/Intercept is the party's
// MASKED weighted contribution (n_k·x_k + mask), and N is its clear sample count.
func (MaskedSumAggregator) Aggregate(partials []Partial) ([]byte, error) {
	if len(partials) == 0 {
		return nil, ErrNoPartials
	}
	dim := len(partials[0].Weights)
	totalN := 0
	for _, p := range partials {
		if len(p.Weights) != dim {
			return nil, ErrDimMismatch
		}
		totalN += p.N
	}
	if totalN <= 0 {
		return nil, ErrZeroSamples
	}
	// Sum the masked contributions; pairwise masks cancel in the total.
	sumW := make([]float64, dim)
	var sumI float64
	for _, p := range partials {
		for i := range sumW {
			sumW[i] += p.Weights[i]
		}
		sumI += p.Intercept
	}
	// Divide by the total sample count → weighted FedAvg of the (now unmasked) sum.
	w := make([]float64, dim)
	for i := range w {
		w[i] = sumW[i] / float64(totalN)
	}
	out := map[string]any{
		"_format":      "fedmodel-v1",
		"weights":      w,
		"intercept":    sumI / float64(totalN),
		"n_total":      totalN,
		"participants": len(partials),
		"aggregation":  "masked-sum",
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("compute: marshal joint model: %w", err)
	}
	return b, nil
}

// parsePartial decodes a sub-job's fedparams-v1 output into a Partial.
func parsePartial(raw []byte) (Partial, error) {
	var p struct {
		Format    string    `json:"_format"`
		Weights   []float64 `json:"weights"`
		Intercept float64   `json:"intercept"`
		N         int       `json:"n"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return Partial{}, fmt.Errorf("compute: parse partial: %w", err)
	}
	if p.Format != "fedparams-v1" {
		return Partial{}, fmt.Errorf("compute: unexpected partial format %q", p.Format)
	}
	return Partial{Weights: p.Weights, Intercept: p.Intercept, N: p.N}, nil
}
