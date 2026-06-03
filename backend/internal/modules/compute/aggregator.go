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
// Implementations: FedAvgAggregator (weighted mean). MPCAggregator is reserved
// for P4-c (secure multi-party computation) and is not implemented in the MVP.
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
