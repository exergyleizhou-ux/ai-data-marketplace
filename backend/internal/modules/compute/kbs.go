package compute

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
)

// --- attestation-based key release (KBS) — design P3 §4 / Direction B §4 ---
//
// What makes L2 "invisible even to the platform" is NOT the sandbox (the platform
// operator can read sandbox data); it is that the dataset is encrypted at rest and
// its decryption key is released into the compute environment ONLY after a remote
// attestation proves the approved algorithm runs in a genuine TEE. The Key Broker
// Service (KBS) is the gate: no valid attestation ⇒ no key ⇒ no plaintext.
//
// A real KBS verifies a hardware quote (Intel TDX / AMD SEV-SNP via DCAP / a cloud
// attestation service) and releases the seller's wrapped data key (decryptable only
// inside the enclave). MockKBS is a non-hardware stand-in: it verifies the (HMAC)
// MockAttester report, enforces a measurement allowlist, and returns a deterministic
// stand-in key — so the GATE (release vs deny) is exercised end-to-end without TEE
// hardware. The real KBS is gated like the dockerRunner.

var (
	// ErrAttestationInvalid means the attestation report did not verify (forged or
	// tampered) — the key broker releases nothing.
	ErrAttestationInvalid = errors.New("compute: attestation did not verify")
	// ErrMeasurementNotAllowed means the attestation is genuine but its measurement
	// (the algorithm image digest) is not in the dataset's key-release policy.
	ErrMeasurementNotAllowed = errors.New("compute: measurement not in key-release policy")
)

// KeyBroker releases a dataset's decryption key into a compute environment only
// after a valid remote attestation whose measurement is in the release policy.
type KeyBroker interface {
	// ReleaseDataKey verifies the attestation report and, iff it is genuine AND its
	// measurement is permitted, returns the dataset's decryption key. Any failure
	// returns a non-nil error and NO key — the caller must then refuse data access.
	ReleaseDataKey(ctx context.Context, report []byte, datasetID string) ([]byte, error)
}

// mockKBS is a non-hardware KeyBroker: it verifies a MockAttester report, enforces
// a measurement allowlist, and derives a deterministic stand-in data key.
type mockKBS struct {
	verifier Attester
	allowed  map[string]struct{} // measurement allowlist; empty ⇒ accept any verified measurement (dev mode)
}

// NewMockKBS returns a mock KeyBroker. verifier checks the attestation (defaults to
// a MockAttester). allowedMeasurements is the release policy: when non-empty, only
// those algorithm image digests get a key; when empty, any verified measurement
// passes (dev/skeleton). A real deployment must populate this from approved algos.
func NewMockKBS(verifier Attester, allowedMeasurements ...string) KeyBroker {
	if verifier == nil {
		verifier = NewMockAttester()
	}
	allow := make(map[string]struct{}, len(allowedMeasurements))
	for _, m := range allowedMeasurements {
		allow[m] = struct{}{}
	}
	return &mockKBS{verifier: verifier, allowed: allow}
}

// ReleaseDataKey implements KeyBroker (fail-closed: any doubt ⇒ no key).
func (k *mockKBS) ReleaseDataKey(ctx context.Context, report []byte, datasetID string) ([]byte, error) {
	a, err := k.verifier.Verify(ctx, report)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationInvalid, err)
	}
	if !a.Verified {
		return nil, ErrAttestationInvalid
	}
	if a.Measurement == "" {
		return nil, fmt.Errorf("%w: empty measurement", ErrMeasurementNotAllowed)
	}
	if len(k.allowed) > 0 {
		if _, ok := k.allowed[a.Measurement]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrMeasurementNotAllowed, a.Measurement)
		}
	}
	// Derive the dataset's data key. A real KBS returns the seller's wrapped DEK
	// (decryptable only inside the enclave); here we derive a deterministic stand-in
	// so the same dataset always yields the same key and different datasets differ.
	sum := sha256.Sum256([]byte("vo-mock-kbs-dek|" + datasetID))
	return sum[:], nil
}
