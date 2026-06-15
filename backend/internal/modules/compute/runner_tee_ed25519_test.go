package compute

import (
	"context"
	"encoding/json"
	"testing"
)

// TestEd25519AttestVerifyRoundTrip: a signed attestation verifies under the
// matching public key, with the bound fields preserved.
func TestEd25519AttestVerifyRoundTrip(t *testing.T) {
	att, err := NewEd25519Attester()
	if err != nil {
		t.Fatalf("new attester: %v", err)
	}
	in := AttestInput{Measurement: "sha256:abc", JobID: "job-1", OutputSHA: "deadbeef"}
	report, err := att.Attest(context.Background(), in)
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	got, err := att.Verify(context.Background(), report)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !got.Verified {
		t.Fatal("a genuine signature must verify")
	}
	if got.Measurement != in.Measurement || got.JobID != in.JobID || got.OutputSHA != in.OutputSHA {
		t.Fatalf("bound fields not preserved: %+v", got)
	}
}

// TestEd25519VerifyRejectsTamper: changing any bound field invalidates the
// signature (the verifier recomputes the binding and checks it).
func TestEd25519VerifyRejectsTamper(t *testing.T) {
	att, _ := NewEd25519Attester()
	report, _ := att.Attest(context.Background(), AttestInput{Measurement: "sha256:abc", JobID: "job-1", OutputSHA: "out"})
	var a Attestation
	_ = json.Unmarshal(report, &a)
	a.Measurement = "sha256:evil" // attacker swaps the code that supposedly ran
	tampered, _ := json.Marshal(a)
	got, err := att.Verify(context.Background(), tampered)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Verified {
		t.Fatal("tampered measurement must fail verification")
	}
}

// TestEd25519VerifyRejectsWrongKey is the asymmetric property the HMAC mock
// lacks: a verifier trusting a DIFFERENT public key cannot be fooled — you
// cannot forge an attestation without the private (TEE) key.
func TestEd25519VerifyRejectsWrongKey(t *testing.T) {
	signer, _ := NewEd25519Attester()
	report, _ := signer.Attest(context.Background(), AttestInput{Measurement: "sha256:abc", JobID: "j", OutputSHA: "o"})

	other, _ := NewEd25519Attester() // a different root of trust
	verifier := NewEd25519Verifier(other.PublicKey())
	got, err := verifier.Verify(context.Background(), report)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Verified {
		t.Fatal("attestation signed by an untrusted key must NOT verify")
	}
}

// TestEd25519MeasurementPolicy: even a validly-signed attestation is rejected if
// its measurement is not in the trusted allowlist (DCAP-style policy check).
func TestEd25519MeasurementPolicy(t *testing.T) {
	signer, _ := NewEd25519Attester()
	report, _ := signer.Attest(context.Background(), AttestInput{Measurement: "sha256:unapproved", JobID: "j", OutputSHA: "o"})

	verifier := NewEd25519Verifier(signer.PublicKey(), "sha256:approved-only")
	got, err := verifier.Verify(context.Background(), report)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Verified {
		t.Fatal("a measurement outside the allowlist must not verify, even with a valid signature")
	}
}
