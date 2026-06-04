package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// --- secure multi-party computation (MPC / PSI) — Direction D 阶段1 ---
//
// Federated learning (FedAvg) handles "each party trains on its own rows, then we
// average". Some valuable cases can't be cut that way: private set intersection
// (PSI) — joint risk lists, ad attribution — where parties want the OVERLAP of
// their sets without revealing the non-overlapping members. That needs MPC: the
// parties compute over encrypted inputs so no one (and not the platform) sees
// another party's private elements.
//
// 诚实边界 (honest scope, 阶段1): mockMPC computes the intersection IN-PROCESS —
// the platform sees every party's raw set. It is NOT cryptographically private;
// it exists so the orchestration, entitlement checks, and result-gating can be
// built and tested locally with correct PSI SEMANTICS. The real privacy comes in
// 阶段2 by delegating to a mature framework (Secretflow / SPU; the platform stays
// an orchestrator, never holding plaintext). This mirrors how the federated MVP
// shipped on MockRunner first, then a real training image. See
// docs/superpowers/specs/2026-06-04-direction-d-mpc-psi-design.md.

// ErrMPCParties is returned when fewer than two parties are supplied.
var ErrMPCParties = errors.New("compute: MPC/PSI requires at least two parties")

// PSIResult is the outcome of a private set intersection: only the elements
// present in EVERY party's set (never any party's non-intersecting members).
type PSIResult struct {
	Intersection []string // sorted, deduplicated
	Cardinality  int
}

// MPCOrchestrator coordinates a secure multi-party computation and returns ONLY
// the agreed result, never the raw inputs. Implementations:
//   - mockMPC      — in-process (阶段1; platform-visible, NOT private; for local dev/tests).
//   - secretflowMPC — delegates to Secretflow/SPU (阶段2; real cryptographic privacy).
type MPCOrchestrator interface {
	// RunPSI returns the intersection of N (>=2) parties' sets.
	RunPSI(ctx context.Context, parties [][]string) (PSIResult, error)
	Kind() string
}

// mockMPC is a non-cryptographic, in-process MPCOrchestrator with correct PSI
// semantics — the orchestration/result shape without the privacy (阶段1).
type mockMPC struct{}

// NewMockMPC returns an in-process mock MPC orchestrator.
func NewMockMPC() MPCOrchestrator { return mockMPC{} }

// Kind identifies the orchestrator in audit/metrics.
func (mockMPC) Kind() string { return "mock" }

// RunPSI computes the set intersection across all parties: an element is in the
// result iff it appears in every party's set. The result is deduplicated and
// sorted for deterministic output.
func (mockMPC) RunPSI(_ context.Context, parties [][]string) (PSIResult, error) {
	if len(parties) < 2 {
		return PSIResult{}, ErrMPCParties
	}
	// Count, per element, how many DISTINCT parties contain it. An element shared
	// by all parties has a count equal to the number of parties.
	seenByParty := make(map[string]int)
	for _, set := range parties {
		// Dedupe within a party so repeats don't inflate the cross-party count.
		distinct := make(map[string]struct{}, len(set))
		for _, e := range set {
			distinct[e] = struct{}{}
		}
		for e := range distinct {
			seenByParty[e]++
		}
	}
	inter := make([]string, 0)
	for e, n := range seenByParty {
		if n == len(parties) {
			inter = append(inter, e)
		}
	}
	sort.Strings(inter)
	return PSIResult{Intersection: inter, Cardinality: len(inter)}, nil
}

// parsePSISet decodes a sub-job's psi-set-v1 output into a party's element set.
func parsePSISet(raw []byte) ([]string, error) {
	var p struct {
		Format   string   `json:"_format"`
		Elements []string `json:"elements"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("compute: parse psi set: %w", err)
	}
	if p.Format != "psi-set-v1" {
		return nil, fmt.Errorf("compute: unexpected psi set format %q", p.Format)
	}
	return p.Elements, nil
}

// marshalPSIResult renders a PSI intersection as the buyer-facing joint output.
func marshalPSIResult(res PSIResult, participants int) ([]byte, error) {
	out := map[string]any{
		"_format":      "psi-result-v1",
		"intersection": res.Intersection,
		"cardinality":  res.Cardinality,
		"participants": participants,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("compute: marshal psi result: %w", err)
	}
	return b, nil
}
