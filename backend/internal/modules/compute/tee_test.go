package compute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestMockAttester_AttestVerify(t *testing.T) {
	ctx := context.Background()
	att := NewMockAttester()
	report, err := att.Attest(ctx, AttestInput{Measurement: "sha256:abc", JobID: "job-1", OutputSHA: "deadbeef"})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	a, err := att.Verify(ctx, report)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !a.Verified {
		t.Fatal("genuine report should verify")
	}
	if a.Measurement != "sha256:abc" || a.JobID != "job-1" || a.OutputSHA != "deadbeef" || a.Signer != "mock-tee" {
		t.Fatalf("attestation not bound correctly: %+v", a)
	}
}

func TestMockAttester_TamperDetected(t *testing.T) {
	ctx := context.Background()
	att := NewMockAttester()
	report, _ := att.Attest(ctx, AttestInput{Measurement: "sha256:abc", JobID: "job-1", OutputSHA: "deadbeef"})
	// Tamper with the bound output hash (an attacker swapping the output).
	var m map[string]any
	_ = json.Unmarshal(report, &m)
	m["output_sha"] = "00000000"
	tampered, _ := json.Marshal(m)
	a, err := att.Verify(ctx, tampered)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if a.Verified {
		t.Fatal("tampered report must NOT verify")
	}
}

func TestMockAttester_RequiresMeasurement(t *testing.T) {
	if _, err := NewMockAttester().Attest(context.Background(), AttestInput{JobID: "j"}); err == nil {
		t.Fatal("attestation without a measurement must fail")
	}
}

func TestTEERunner_BindsAttestationToCodeAndOutput(t *testing.T) {
	ctx := context.Background()
	att := NewMockAttester()
	r := NewTEERunner(NewMockRunner(), att)
	if r.Kind() != "tee:mock" {
		t.Fatalf("kind = %q", r.Kind())
	}
	req := RunRequest{
		Job:       Job{ID: "job-9", Params: nil},
		Algorithm: Algorithm{Name: "logreg", ImageDigest: "sha256:codedigest", OutputKind: OutputModel},
	}
	res, err := r.Run(ctx, req)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Attestation) == 0 {
		t.Fatal("tee runner must attach an attestation")
	}
	a, err := att.Verify(ctx, res.Attestation)
	if err != nil || !a.Verified {
		t.Fatalf("attestation should verify: %+v err=%v", a, err)
	}
	// Bound to WHAT ran (the algorithm digest) and the exact output.
	sum := sha256.Sum256(res.Output)
	if a.Measurement != "sha256:codedigest" || a.JobID != "job-9" || a.OutputSHA != hex.EncodeToString(sum[:]) {
		t.Fatalf("attestation binding wrong: %+v", a)
	}
}
