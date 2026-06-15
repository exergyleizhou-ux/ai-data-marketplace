package compute

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sort"
)

// --- real Diffie-Hellman PSI (Direction D 阶段2 primitive) ---
//
// Replaces mockMPC's "platform sees every party's plaintext" with a genuinely
// cryptographic intersection. DDH-PSI rests on the Decisional Diffie-Hellman
// assumption in a prime-order group: each party blinds its hashed elements with
// a private exponent; because exponentiation commutes, an element shared by all
// parties maps to the IDENTICAL fully-blinded group element regardless of order,
// while the blinded points reveal nothing about the plaintext to whoever routes
// them. The orchestrator only ever compares blinded big.Int values — never raw
// elements.
//
// Group: the RFC 3526 2048-bit MODP safe prime p = 2q+1. Hashing squares the
// SHA-256 digest into the order-q quadratic-residue subgroup; secrets live in
// [2, q). Equality of H(z)^K (K = product of all secrets) decides membership.
//
// 诚实边界 (honest scope): this is a semi-honest 2..N-party protocol. It reveals
// the intersection (the point) and the parties' set sizes; it assumes parties
// follow the protocol (no malicious-security / cardinality hiding). Here the N
// parties run in one process for local verification — a real deployment runs each
// party's blinding inside its own sandbox/node (Secretflow/SPU) so the platform
// only relays blinded points. The cryptography below is real; the multi-node
// transport is the remaining integration. See
// docs/superpowers/specs/2026-06-04-direction-d-mpc-psi-design.md.

// psiPrime is the RFC 3526 2048-bit MODP group prime (a safe prime, p = 2q+1).
var psiPrime, _ = new(big.Int).SetString(
	"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74"+
		"020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F14374"+
		"FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7EDEE"+
		"386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF0598D"+
		"A48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB9ED5"+
		"29077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3BE39E7"+
		"72C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF6955817183995497"+
		"CEA956AE515D2261898FA051015728E5A8AACAA68FFFFFFFFFFFFFFFF", 16)

// psiQ = (p-1)/2 is the order of the quadratic-residue subgroup.
var psiQ = new(big.Int).Rsh(new(big.Int).Sub(psiPrime, big.NewInt(1)), 1)

var psiTwo = big.NewInt(2)

// newPSISecret draws a private exponent uniformly from [2, q).
func newPSISecret() (*big.Int, error) {
	// rand.Int gives [0, q-2); shift to [2, q).
	max := new(big.Int).Sub(psiQ, psiTwo)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("compute: psi secret: %w", err)
	}
	return n.Add(n, psiTwo), nil
}

// psiHashToGroup maps an element into the order-q subgroup: square the SHA-256
// digest mod p so the result is a quadratic residue (hence in the prime-order
// subgroup), independent of the element's representation.
func psiHashToGroup(elem string) *big.Int {
	h := sha256.Sum256([]byte(elem))
	x := new(big.Int).SetBytes(h[:])
	x.Mod(x, psiPrime)
	return x.Exp(x, psiTwo, psiPrime) // h^2 mod p ∈ QR subgroup
}

// psiBlind raises a group element to a secret exponent (mod p).
func psiBlind(point, secret *big.Int) *big.Int {
	return new(big.Int).Exp(point, secret, psiPrime)
}

// ddhPSI is the real cryptographic MPCOrchestrator.
type ddhPSI struct{}

// NewDDHPSI returns a Diffie-Hellman PSI orchestrator.
func NewDDHPSI() MPCOrchestrator { return ddhPSI{} }

// Kind identifies the orchestrator in audit/metrics.
func (ddhPSI) Kind() string { return "ddh-psi" }

// RunPSI computes the intersection by fully blinding every party's set with ALL
// parties' secrets and matching the resulting group elements. Party 0 keeps the
// alignment between its plaintext set and its fully-blinded points so it can
// label the result; the matching itself is over blinded big.Int values only.
func (ddhPSI) RunPSI(_ context.Context, parties [][]string) (PSIResult, error) {
	if len(parties) < 2 {
		return PSIResult{}, ErrMPCParties
	}
	secrets := make([]*big.Int, len(parties))
	for i := range secrets {
		s, err := newPSISecret()
		if err != nil {
			return PSIResult{}, err
		}
		secrets[i] = s
	}

	// fullyBlind hashes each element into the group then applies every party's
	// secret. Because exponentiation commutes, the order does not matter and a
	// shared element yields the identical point from any party.
	fullyBlind := func(set []string) []*big.Int {
		out := make([]*big.Int, len(set))
		for j, e := range set {
			pt := psiHashToGroup(e)
			for _, k := range secrets {
				pt = psiBlind(pt, k)
			}
			out[j] = pt
		}
		return out
	}

	key := func(x *big.Int) string { return x.Text(16) }

	// Every party except 0 contributes a set of fully-blinded points to match against.
	otherSets := make([]map[string]struct{}, 0, len(parties)-1)
	for i := 1; i < len(parties); i++ {
		m := make(map[string]struct{})
		for _, p := range fullyBlind(parties[i]) {
			m[key(p)] = struct{}{}
		}
		otherSets = append(otherSets, m)
	}

	// Walk party 0's elements; an element is in the intersection iff its
	// fully-blinded point appears in every other party's blinded set.
	blinded0 := fullyBlind(parties[0])
	seen := make(map[string]struct{})
	inter := make([]string, 0)
	for j, e := range parties[0] {
		if _, dup := seen[e]; dup {
			continue
		}
		k := key(blinded0[j])
		inAll := true
		for _, m := range otherSets {
			if _, ok := m[k]; !ok {
				inAll = false
				break
			}
		}
		if inAll {
			seen[e] = struct{}{}
			inter = append(inter, e)
		}
	}
	sort.Strings(inter)
	return PSIResult{Intersection: inter, Cardinality: len(inter)}, nil
}
