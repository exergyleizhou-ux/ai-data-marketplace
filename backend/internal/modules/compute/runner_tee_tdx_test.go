package compute

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// On any non-TDX machine (dev, CI), the TDX attester must fail CLOSED: if the
// platform is configured for hardware attestation but /dev/tdx_guest is absent,
// it must refuse — never silently produce a fake/empty quote that would let an
// L2 job run without real TEE protection.
func TestTDXAttester_FailsClosedWithoutHardware(t *testing.T) {
	_, err := NewTDXAttester().Attest(context.Background(), AttestInput{
		Measurement: "sha256:abc", JobID: "job-1", OutputSHA: "deadbeef",
	})
	if !errors.Is(err, ErrTEEUnavailable) {
		t.Fatalf("without /dev/tdx_guest, Attest must return ErrTEEUnavailable, got %v", err)
	}
}

// A TEE runner backed by the TDX attester must also fail closed off-hardware:
// the job errors and produces NO output (no data egress without attestation).
func TestTEERunner_TDXFailsClosedWithoutHardware(t *testing.T) {
	r := NewTEERunner(NewMockRunner(), NewTDXAttester())
	_, err := r.Run(context.Background(), RunRequest{
		Job:       Job{ID: "job-9"},
		Algorithm: Algorithm{Name: "logreg", ImageDigest: "sha256:codedigest", OutputKind: OutputModel},
	})
	if err == nil {
		t.Fatal("tee runner with TDX attester must fail closed when no TEE hardware is present")
	}
}

// Verify honestly reports that hardware-quote genuineness is established by the
// KBS/DCAP, not in-process: it parses the envelope and returns Verified=false
// with the tdx signer (the buyer-facing trust comes from the KBS release, not
// this display path). This documents the trust boundary rather than overclaiming.
func TestTDXAttester_VerifyHonestlyDefersToDCAP(t *testing.T) {
	env, _ := json.Marshal(Attestation{
		Format: "tdx-quote-1", Measurement: "sha256:abc", JobID: "j", OutputSHA: "d", Quote: "AAAA", Signer: "tdx",
	})
	a, err := NewTDXAttester().Verify(context.Background(), env)
	if err != nil {
		t.Fatalf("verify parse: %v", err)
	}
	if a.Signer != "tdx" {
		t.Fatalf("signer = %q, want tdx", a.Signer)
	}
	if a.Verified {
		t.Fatal("in-process Verify must NOT claim a hardware quote is verified (DCAP/KBS is the trust path)")
	}
}
