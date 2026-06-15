package compute

import (
	"math/big"
	"sort"
)

// --- two-round DDH PSI with persistent parties (Direction D, the honest target) ---
//
// ddhPSI (psi_ddh.go) computes a cryptographic intersection, but its RunPSI takes
// every party's PLAINTEXT set — so the orchestrator process still momentarily
// holds plaintext. To remove plaintext from the platform entirely, each party must
// hold its own set AND secret and reveal only BLINDED points. That cannot be done
// with stateless, single-shot sandboxes: the protocol is two rounds and a party
// must keep its secret BETWEEN them. Hence persistent party agents.
//
// Protocol (semi-honest, DDH):
//   round 1 — party i publishes A_i = { H(x)^{k_i} : x ∈ S_i }   (blinded, no plaintext)
//   round 2 — the orchestrator routes each A_i through every OTHER party, who
//             raises it to its own secret; after all parties, A_i becomes
//             F_i = { H(x)^{Πk} } (fully blinded by the product of all secrets)
//   match  — F_i are comparable across parties (same exponent Πk); the result
//             party maps its own matching points back to plaintext.
//
// The orchestrator only ever sees blinded group elements; no party sees another's
// set or secret. 诚实边界: each psiPartyAgent stands in for a long-lived party
// PROCESS (a sandbox/node holding state across rounds). Running them in-process
// here verifies the protocol end-to-end; productionizing places each agent in its
// own sandbox and routes round1()/reblind() over the wire — that long-lived,
// multi-node transport is the remaining integration, not more cryptography.

// psiPartyAgent is a stateful PSI participant: it holds its set and secret
// privately and exposes only blinded-point operations.
type psiPartyAgent struct {
	secret *big.Int
	set    []string
}

// newPSIPartyAgent creates a party holding the given set with a fresh secret.
func newPSIPartyAgent(set []string) (*psiPartyAgent, error) {
	s, err := newPSISecret()
	if err != nil {
		return nil, err
	}
	return &psiPartyAgent{secret: s, set: set}, nil
}

// round1 returns the party's set hashed-to-group and blinded with its own secret.
// Plaintext never leaves the agent.
func (p *psiPartyAgent) round1() []*big.Int {
	out := make([]*big.Int, len(p.set))
	for i, e := range p.set {
		out[i] = psiBlind(psiHashToGroup(e), p.secret)
	}
	return out
}

// reblind raises peers' points to this party's secret (the round-2 contribution).
func (p *psiPartyAgent) reblind(points []*big.Int) []*big.Int {
	out := make([]*big.Int, len(points))
	for i, pt := range points {
		out[i] = psiBlind(pt, p.secret)
	}
	return out
}

// elementsAt returns this party's plaintext elements at the given indices — used
// only by the result party to label ITS OWN intersection members.
func (p *psiPartyAgent) elementsAt(idx []int) []string {
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, p.set[i])
	}
	return out
}

// runTwoRoundPSI executes the protocol over persistent party agents. The
// orchestrator holds only blinded points; party 0 (the result party) labels the
// intersection from its own set.
func runTwoRoundPSI(agents []*psiPartyAgent) (PSIResult, error) {
	if len(agents) < 2 {
		return PSIResult{}, ErrMPCParties
	}
	// Round 1: every party blinds its own set.
	blinded := make([][]*big.Int, len(agents))
	for i, a := range agents {
		blinded[i] = a.round1()
	}
	// Round 2: route each party's points through every OTHER party so they end up
	// raised by the product of all secrets (order-independent — exponents commute).
	fully := make([][]*big.Int, len(agents))
	for i := range agents {
		pts := blinded[i]
		for j := range agents {
			if j != i {
				pts = agents[j].reblind(pts)
			}
		}
		fully[i] = pts
	}

	key := func(x *big.Int) string { return x.Text(16) }
	// Sets of fully-blinded points for parties 1..n-1.
	otherSets := make([]map[string]struct{}, 0, len(agents)-1)
	for i := 1; i < len(agents); i++ {
		m := make(map[string]struct{}, len(fully[i]))
		for _, p := range fully[i] {
			m[key(p)] = struct{}{}
		}
		otherSets = append(otherSets, m)
	}

	// An element of party 0 is in the intersection iff its fully-blinded point is
	// present in every other party's fully-blinded set.
	seen := make(map[string]struct{})
	matchIdx := make([]int, 0)
	for idx := range fully[0] {
		k := key(fully[0][idx])
		inAll := true
		for _, m := range otherSets {
			if _, ok := m[k]; !ok {
				inAll = false
				break
			}
		}
		if !inAll {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		matchIdx = append(matchIdx, idx)
	}

	inter := agents[0].elementsAt(matchIdx)
	sort.Strings(inter)
	return PSIResult{Intersection: inter, Cardinality: len(inter)}, nil
}
