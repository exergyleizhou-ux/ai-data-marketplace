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
// Two implementations exist:
//   - mockMPC — computes the intersection IN-PROCESS over plaintext; NOT private
//     (the platform sees every party's raw set). Kept for fast tests/dev and as
//     the correctness oracle for the real one.
//   - ddhPSI (psi_ddh.go) — REAL Diffie-Hellman PSI: parties blind their hashed
//     elements with private exponents, and the orchestrator matches only blinded
//     group elements, never plaintext (semi-honest, DDH assumption). This closes
//     the "platform sees plaintext" gap cryptographically.
//
// 诚实边界 (honest scope): ddhPSI is the real primitive; what remains for a
// production deployment is running each party's blinding inside its own
// sandbox/node (Secretflow/SPU transport) and malicious-security / cardinality
// hiding. Until then the N parties are blinded in one process for local
// verification. See docs/superpowers/specs/2026-06-04-direction-d-mpc-psi-design.md.

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
