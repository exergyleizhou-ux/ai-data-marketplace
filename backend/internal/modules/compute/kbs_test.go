package compute

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// validReport produces a genuine MockAttester report bound to the given
// measurement, as an enclave would present to the key broker before data access.
func validReport(t *testing.T, att Attester, measurement string) []byte {
	t.Helper()
	report, err := att.Attest(context.Background(), AttestInput{Measurement: measurement, JobID: "job-kbs"})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	return report
}

func TestMockKBS_ReleasesKeyOnValidAttestation(t *testing.T) {
	att := NewMockAttester()
	kbs := NewMockKBS(att, "sha256:approved")
	key, err := kbs.ReleaseDataKey(context.Background(), validReport(t, att, "sha256:approved"), "ds-1")
	if err != nil {
		t.Fatalf("valid attestation in policy should release a key: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("released key must be non-empty")
	}
}

func TestMockKBS_RefusesTamperedAttestation(t *testing.T) {
	att := NewMockAttester()
	report := validReport(t, att, "sha256:approved")
	// Tamper: swap the measurement so the HMAC no longer matches.
	var m map[string]any
	_ = json.Unmarshal(report, &m)
	m["measurement"] = "sha256:evil"
	tampered, _ := json.Marshal(m)

	_, err := NewMockKBS(att, "sha256:approved", "sha256:evil").ReleaseDataKey(context.Background(), tampered, "ds-1")
	if !errors.Is(err, ErrAttestationInvalid) {
		t.Fatalf("tampered attestation must be refused with ErrAttestationInvalid, got %v", err)
	}
}

func TestMockKBS_RefusesMeasurementNotInPolicy(t *testing.T) {
	att := NewMockAttester()
	// Genuine attestation, but its measurement is NOT in the release policy.
	_, err := NewMockKBS(att, "sha256:approved").ReleaseDataKey(context.Background(), validReport(t, att, "sha256:other"), "ds-1")
	if !errors.Is(err, ErrMeasurementNotAllowed) {
		t.Fatalf("measurement outside policy must be refused with ErrMeasurementNotAllowed, got %v", err)
	}
}

func TestMockKBS_AcceptsAnyMeasurementWhenPolicyEmpty(t *testing.T) {
	att := NewMockAttester()
	// Empty policy = dev mode: any genuinely-attested measurement is accepted.
	key, err := NewMockKBS(att).ReleaseDataKey(context.Background(), validReport(t, att, "sha256:whatever"), "ds-1")
	if err != nil {
		t.Fatalf("empty policy should accept any verified measurement: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("released key must be non-empty")
	}
}

func TestMockKBS_DeterministicKeyPerDataset(t *testing.T) {
	att := NewMockAttester()
	kbs := NewMockKBS(att)
	k1, _ := kbs.ReleaseDataKey(context.Background(), validReport(t, att, "m"), "ds-1")
	k1again, _ := kbs.ReleaseDataKey(context.Background(), validReport(t, att, "m"), "ds-1")
	k2, _ := kbs.ReleaseDataKey(context.Background(), validReport(t, att, "m"), "ds-2")
	if string(k1) != string(k1again) {
		t.Fatal("same dataset must yield the same data key")
	}
	if string(k1) == string(k2) {
		t.Fatal("different datasets must yield different data keys")
	}
}

// callRecordingRunner records whether Run was invoked, to prove the KBS gate
// fails CLOSED (no data access when the key is denied).
type callRecordingRunner struct{ ran bool }

func (r *callRecordingRunner) Kind() string          { return "spy" }
func (r *callRecordingRunner) NeedsStagedData() bool { return false }
func (r *callRecordingRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	r.ran = true
	return RunResult{OutputKind: req.Algorithm.OutputKind, Output: []byte("{}")}, nil
}

func TestTEERunner_KBSGate_AllowsRunWhenKeyReleased(t *testing.T) {
	att := NewMockAttester()
	spy := &callRecordingRunner{}
	r := NewTEERunnerWithKBS(spy, att, NewMockKBS(att, "sha256:codedigest"))
	res, err := r.Run(context.Background(), RunRequest{
		Job:       Job{ID: "job-9", DatasetID: "ds-1"},
		Algorithm: Algorithm{Name: "logreg", ImageDigest: "sha256:codedigest", OutputKind: OutputModel},
	})
	if err != nil {
		t.Fatalf("run with a valid KBS release should succeed: %v", err)
	}
	if !spy.ran {
		t.Fatal("base runner must execute after the key is released")
	}
	if len(res.Attestation) == 0 {
		t.Fatal("tee runner must still attach an output attestation")
	}
}

func TestTEERunner_KBSGate_FailsClosedWhenKeyDenied(t *testing.T) {
	att := NewMockAttester()
	spy := &callRecordingRunner{}
	// Policy excludes the algorithm's digest → key release denied.
	r := NewTEERunnerWithKBS(spy, att, NewMockKBS(att, "sha256:somethingelse"))
	_, err := r.Run(context.Background(), RunRequest{
		Job:       Job{ID: "job-9", DatasetID: "ds-1"},
		Algorithm: Algorithm{Name: "logreg", ImageDigest: "sha256:codedigest", OutputKind: OutputModel},
	})
	if err == nil {
		t.Fatal("run must fail when the KBS denies the data key")
	}
	if spy.ran {
		t.Fatal("base runner must NOT execute when the key is denied (fail closed — no data access)")
	}
}
