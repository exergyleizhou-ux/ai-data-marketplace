package compute

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// --- asymmetric software attester (design P3 / L2, off-hardware) ---
//
// Ed25519Attester models the trust shape of REAL remote attestation that the
// symmetric MockAttester cannot: asymmetry. With HMAC, whoever can verify can
// also forge (same secret). Real attestation is asymmetric — a verifier trusts
// only a PUBLIC root (Intel's DCAP root, an AMD SEV key) and can check a quote's
// signature without any power to forge one. Ed25519Attester signs the
// (measurement|job|output) binding with a private key; Verify checks it with the
// public key alone, and additionally enforces a measurement allowlist (the
// DCAP-style policy: "only approved code may be attested").
//
// HONEST SCOPE: the software keypair stands in for the hardware root of trust.
// In production the signer is the TEE/DCAP chain (a quote signed under a
// hardware-rooted key whose cert chains to Intel/AMD), and the public key here is
// replaced by that root + collateral. The verification SEMANTICS — recompute the
// binding, check the signature against a trusted public key, enforce the
// measurement policy — are exactly the real ones and are what this exercises.
type Ed25519Attester struct {
	priv  ed25519.PrivateKey  // signer ("TEE"); nil for a verify-only instance
	pub   ed25519.PublicKey   // the trusted verification key
	allow map[string]struct{} // trusted measurements; empty = signature-only (no policy)
}

// NewEd25519Attester generates a fresh dev keypair (signer + verifier).
func NewEd25519Attester() (*Ed25519Attester, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("compute: ed25519 keygen: %w", err)
	}
	return &Ed25519Attester{priv: priv, pub: pub}, nil
}

// NewEd25519Verifier returns a verify-only attester that trusts pub and, if any
// measurements are given, accepts only those (policy allowlist).
func NewEd25519Verifier(pub ed25519.PublicKey, allowedMeasurements ...string) *Ed25519Attester {
	allow := make(map[string]struct{}, len(allowedMeasurements))
	for _, m := range allowedMeasurements {
		allow[m] = struct{}{}
	}
	return &Ed25519Attester{pub: pub, allow: allow}
}

// PublicKey returns the verification key to publish to relying parties.
func (a *Ed25519Attester) PublicKey() ed25519.PublicKey { return a.pub }

// attestBinding is the 32-byte message the signature commits to: the same
// (measurement|job|output) tuple the hardware REPORTDATA would bind.
func attestBinding(in AttestInput) []byte {
	sum := sha256.Sum256([]byte(in.Measurement + "|" + in.JobID + "|" + in.OutputSHA))
	return sum[:]
}

// Attest signs the binding with the private key.
func (a *Ed25519Attester) Attest(_ context.Context, in AttestInput) ([]byte, error) {
	if a.priv == nil {
		return nil, fmt.Errorf("compute: attester has no signing key (verify-only)")
	}
	if in.Measurement == "" {
		return nil, fmt.Errorf("attestation requires a measurement (algorithm image digest)")
	}
	sig := ed25519.Sign(a.priv, attestBinding(in))
	return json.Marshal(Attestation{
		Format: "vo-attest-ed25519-1", Measurement: in.Measurement, JobID: in.JobID,
		OutputSHA: in.OutputSHA, Quote: base64.StdEncoding.EncodeToString(sig), Signer: "ed25519-sw",
	})
}

// Verify recomputes the binding, checks the signature against the trusted public
// key, and enforces the measurement allowlist. Verified is true only when the
// signature is genuine AND (if a policy is set) the measurement is approved.
func (a *Ed25519Attester) Verify(_ context.Context, report []byte) (Attestation, error) {
	var att Attestation
	if err := json.Unmarshal(report, &att); err != nil {
		return Attestation{}, fmt.Errorf("compute: parse attestation: %w", err)
	}
	att.Verified = false
	if len(a.pub) == 0 {
		return att, nil
	}
	sig, err := base64.StdEncoding.DecodeString(att.Quote)
	if err != nil {
		return att, nil // malformed quote → unverified, not an error
	}
	binding := attestBinding(AttestInput{Measurement: att.Measurement, JobID: att.JobID, OutputSHA: att.OutputSHA})
	if !ed25519.Verify(a.pub, binding, sig) {
		return att, nil // bad signature / wrong key / tampered fields
	}
	// Signature is genuine. Enforce the measurement policy if one is configured.
	if len(a.allow) > 0 {
		if _, ok := a.allow[att.Measurement]; !ok {
			return att, nil // signed, but the code that ran is not approved
		}
	}
	att.Verified = true
	return att, nil
}
