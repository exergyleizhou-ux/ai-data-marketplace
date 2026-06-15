package compute

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// secAggParty holds one participant's secure-aggregation key material. Secure
// aggregation (Bonawitz et al. 2017, Direction C) lets N parties contribute their
// local model params to a sum WITHOUT the aggregator ever seeing a single party's
// real params: each pair of parties (i,j) derives a shared mask from an ECDH
// secret; party i adds it and party j subtracts it, so across the whole cohort
// the masks cancel and only the true sum survives.
//
// This file implements the cryptographic core that the math half (MaskedSumAggregator)
// previously ASSUMED: real pairwise masks generated from key agreement, never
// distributed by the platform.
//
// 诚实边界 (honest scope, 阶段2): masks are derived in floating point and cancel
// to within ~1e-9 (production secure aggregation uses fixed-point modular
// arithmetic for exact cancellation). This is the semi-honest ("honest-but-curious"
// aggregator) threat model: the platform may route public keys but learns nothing
// about individual params. Dropout recovery (Shamir shares of masks) and defence
// against a MALICIOUS key-distributing server (authenticated PKI) are 阶段3 — until
// those land, in-sandbox key agreement must run where the platform cannot MITM it.
type secAggParty struct {
	index int
	priv  *ecdh.PrivateKey
}

// secAggDomain separates this construction's PRG from any other use of the
// ECDH secret, per standard domain-separation hygiene.
const secAggDomain = "oasis-secagg-v1"

// secAggMaskRange bounds the per-coordinate mask magnitude. Large relative to
// real params (which are O(1)–O(100)) so a single masked value reveals nothing
// useful, but finite so float cancellation stays well within tolerance.
const secAggMaskRange = 1 << 20

// newSecAggParty generates a fresh X25519 keypair for the given cohort index.
func newSecAggParty(index int) (*secAggParty, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("compute: secagg keygen: %w", err)
	}
	return &secAggParty{index: index, priv: priv}, nil
}

// pubKey returns the party's public key bytes to publish to the cohort.
func (p *secAggParty) pubKey() []byte { return p.priv.PublicKey().Bytes() }

// mask returns the party's weighted contribution with pairwise masks added. For
// each peer it derives the shared ECDH secret, expands it into a deterministic
// mask vector, and adds it with a sign fixed by index order (+ when this party's
// index is lower, − when higher) so that for every pair the two contributions
// cancel. The clear sample count N is left untouched — the aggregator needs it to
// weight the average and it leaks nothing about the params.
func (p *secAggParty) mask(c Partial, peers map[int][]byte) (Partial, error) {
	dim := len(c.Weights)
	out := Partial{
		Weights:   make([]float64, dim),
		Intercept: c.Intercept,
		N:         c.N,
	}
	copy(out.Weights, c.Weights)

	for peerIndex, peerPubBytes := range peers {
		peerPub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
		if err != nil {
			return Partial{}, fmt.Errorf("compute: secagg peer %d pubkey: %w", peerIndex, err)
		}
		shared, err := p.priv.ECDH(peerPub)
		if err != nil {
			return Partial{}, fmt.Errorf("compute: secagg ecdh with peer %d: %w", peerIndex, err)
		}
		// Pairwise masks must be IDENTICAL on both sides, so derive from the
		// shared secret alone (ECDH is symmetric). Sign by index order makes the
		// pair cancel: lower index adds, higher subtracts.
		sign := 1.0
		if p.index > peerIndex {
			sign = -1.0
		}
		m := expandMask(shared, dim+1)
		for i := 0; i < dim; i++ {
			out.Weights[i] += sign * m[i]
		}
		out.Intercept += sign * m[dim]
	}
	return out, nil
}

// expandMask deterministically derives n mask coordinates in (-range, range) from
// a shared secret using SHA-256 in counter mode (a simple PRG). Both parties in a
// pair feed the identical shared secret, so they produce the identical vector.
func expandMask(shared []byte, n int) []float64 {
	out := make([]float64, n)
	seed := sha256.Sum256(append([]byte(secAggDomain+":"), shared...))
	var ctr uint64
	var buf [8]byte
	for i := 0; i < n; i++ {
		binary.BigEndian.PutUint64(buf[:], ctr)
		block := sha256.Sum256(append(seed[:], buf[:]...))
		u := binary.BigEndian.Uint64(block[:8])
		// Map u ∈ [0, 2^64) to a float in [-range, range).
		frac := float64(u) / float64(1<<64) // [0,1)
		out[i] = (frac*2 - 1) * secAggMaskRange
		ctr++
	}
	return out
}
