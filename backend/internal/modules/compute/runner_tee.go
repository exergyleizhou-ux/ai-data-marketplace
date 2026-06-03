package compute

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// --- remote attestation (design P3 / L2) ---
//
// L2 ("confidential computing") promises the data is invisible even to the
// platform: it is decrypted only inside a TEE enclave, and a REMOTE ATTESTATION
// proves to the buyer/seller that "only the approved algorithm code ran, in a
// genuine TEE, over this data". The attestation BINDS three things:
//
//	measurement  — WHAT ran: the algorithm image digest (the audited code)
//	job_id       — freshness/nonce: this specific job (no replay)
//	output_sha   — integrity: the exact output that was released
//
// A real Attester returns a hardware quote (Intel TDX / AMD SEV-SNP / SGX DCAP)
// verified by a cloud/DCAP attestation service. MockAttester signs the binding
// with HMAC-SHA256 under a fixed "TEE signing key" stand-in, so the bind +
// tamper-evidence + independent verify are demonstrated cryptographically
// without TEE hardware (real one is gated like the dockerRunner).

// AttestInput is what an attestation is bound to.
type AttestInput struct {
	Measurement string // algorithm image digest (sha256:...)
	JobID       string
	OutputSHA   string // sha256 hex of the released output
}

// Attestation is a parsed + verified attestation report.
type Attestation struct {
	Format      string `json:"format"`
	Measurement string `json:"measurement"`
	JobID       string `json:"job_id"`
	OutputSHA   string `json:"output_sha"`
	Quote       string `json:"quote"`  // the signature/quote over the binding
	Signer      string `json:"signer"` // who vouches (mock-tee | tdx | sev-snp | sgx-dcap)
	Verified    bool   `json:"verified,omitempty"`
}

// Attester produces and verifies remote-attestation reports.
type Attester interface {
	Attest(ctx context.Context, in AttestInput) ([]byte, error)
	Verify(ctx context.Context, report []byte) (Attestation, error)
}

// MockAttester is a non-TEE stand-in: it HMACs the binding so Verify can detect
// any tampering. NOT a real hardware quote — for tests/dev only.
type MockAttester struct{ key []byte }

// NewMockAttester returns a MockAttester with a fixed dev "signing key".
func NewMockAttester() Attester {
	return MockAttester{key: []byte("vo-mock-tee-signing-key-dev-only")}
}

func (m MockAttester) sign(in AttestInput) string {
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(in.Measurement + "|" + in.JobID + "|" + in.OutputSHA))
	return hex.EncodeToString(mac.Sum(nil))
}

// Attest emits a JSON attestation report binding the measurement/job/output.
func (m MockAttester) Attest(_ context.Context, in AttestInput) ([]byte, error) {
	if in.Measurement == "" {
		return nil, fmt.Errorf("attestation requires a measurement (algorithm image digest)")
	}
	return json.Marshal(Attestation{
		Format: "vo-attest-1", Measurement: in.Measurement, JobID: in.JobID,
		OutputSHA: in.OutputSHA, Quote: m.sign(in), Signer: "mock-tee",
	})
}

// Verify recomputes the HMAC over the report's binding and reports whether it
// matches (tamper-evident). A real verifier would validate the hardware quote
// against the DCAP/cloud attestation service and check the measurement policy.
func (m MockAttester) Verify(_ context.Context, report []byte) (Attestation, error) {
	var a Attestation
	if err := json.Unmarshal(report, &a); err != nil {
		return Attestation{}, fmt.Errorf("parse attestation: %w", err)
	}
	want := m.sign(AttestInput{Measurement: a.Measurement, JobID: a.JobID, OutputSHA: a.OutputSHA})
	a.Verified = hmac.Equal([]byte(want), []byte(a.Quote))
	return a, nil
}

// teeRunner wraps a base Runner (dockerRunner in prod, MockRunner in tests) and
// attaches a remote attestation bound to the algorithm digest + job + output.
// An L2 job that cannot be attested is failed by the worker rather than released
// unattested (design P3).
type teeRunner struct {
	base     Runner
	attester Attester
}

// NewTEERunner wraps base with attestation (zero attester -> MockAttester).
func NewTEERunner(base Runner, attester Attester) Runner {
	if attester == nil {
		attester = NewMockAttester()
	}
	return teeRunner{base: base, attester: attester}
}

func (r teeRunner) Kind() string          { return "tee:" + r.base.Kind() }
func (r teeRunner) NeedsStagedData() bool { return r.base.NeedsStagedData() }

func (r teeRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	res, err := r.base.Run(ctx, req)
	if err != nil {
		return res, err
	}
	sum := sha256.Sum256(res.Output)
	report, err := r.attester.Attest(ctx, AttestInput{
		Measurement: req.Algorithm.ImageDigest,
		JobID:       req.Job.ID,
		OutputSHA:   hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("remote attestation failed: %w", err) // L2: no attestation, no release
	}
	res.Attestation = report
	return res, nil
}

// attestationToMap parses a raw report into a map for JSONB storage; returns nil
// on failure (storage is best-effort metadata).
func attestationToMap(report []byte) map[string]any {
	if len(report) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(report, &m) != nil {
		return nil
	}
	return m
}
