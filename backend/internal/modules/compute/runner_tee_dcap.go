package compute

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-tdx-guest/abi"
	tdxpb "github.com/google/go-tdx-guest/proto/tdx"
	"github.com/google/go-tdx-guest/verify"
)

// --- Real Intel TDX quote verification (design P3 / Direction B, "B2") ---
//
// DCAPVerifier verifies a REAL Intel TDX attestation quote using Google's audited
// go-tdx-guest library, rather than a hand-rolled crypto reimplementation. It
// parses the quote and verifies its ECDSA signature + PCK certificate chain up to
// Intel's SGX Root CA, then extracts the MRTD measurement and checks it against an
// approved-algorithm allowlist (DCAP-style policy).
//
// HONEST SCOPE — what a passing Verify PROVES:
//   - the quote's signature is authentic and its PCK certificate chain validates
//     to Intel's root: this is a genuine, Intel-rooted TDX quote, not a forgery;
//   - the TD measurement (MRTD) matches the approved-algorithm allowlist.
//
// What it does NOT prove (still hardware/collateral-gated — disclosed honestly):
//   - LIVENESS: an offline check cannot distinguish a freshly-emitted quote from
//     a replay. Our design binds freshness via the job id inside REPORTDATA; a
//     live deployment must emit a fresh quote per job on real TDX hardware.
//   - TCB FRESHNESS / REVOCATION: only checked when CheckRevocations+GetCollateral
//     are enabled, which fetch CRLs + TCB/QE-identity from Intel PCS over the
//     network at verify time.
//   - CONFIDENTIALITY: that the operator truly cannot read enclave memory — that
//     is a property of running on genuine TDX hardware, not of verifying a quote.
//
// It implements the Attester interface for VERIFICATION ONLY; producing a quote
// requires TDX hardware (see TDXAttester), so Attest returns an error here. This
// is the real verification half of L2 — it is unit-tested offline against Intel's
// published production sample quote; wiring it to live quotes is gated on hardware.
type DCAPVerifier struct {
	allowed          map[string]struct{}
	now              time.Time
	checkRevocations bool
	getCollateral    bool
}

// DCAPConfig configures a DCAPVerifier.
type DCAPConfig struct {
	// AllowedMeasurements, if non-empty, is the set of acceptable MRTD values
	// (lower-hex). A verified quote whose MRTD is not listed is rejected — the
	// "only approved code ran" policy.
	AllowedMeasurements []string
	// Now overrides the time the PCK certificate validity is checked against.
	// Zero ⇒ time.Now(). (Tests pin it to the sample quote's validity window.)
	Now time.Time
	// CheckRevocations and GetCollateral fetch CRLs + fresh TCB/QE-identity from
	// Intel PCS over the network. Both default false = offline core verification
	// (signature + PCK chain only). Enable in a networked production verifier.
	CheckRevocations bool
	GetCollateral    bool
}

// NewDCAPVerifier builds a verifier from config.
func NewDCAPVerifier(cfg DCAPConfig) *DCAPVerifier {
	allowed := make(map[string]struct{}, len(cfg.AllowedMeasurements))
	for _, m := range cfg.AllowedMeasurements {
		allowed[strings.ToLower(strings.TrimSpace(m))] = struct{}{}
	}
	return &DCAPVerifier{
		allowed:          allowed,
		now:              cfg.Now,
		checkRevocations: cfg.CheckRevocations,
		getCollateral:    cfg.GetCollateral,
	}
}

// Attest is unsupported: a TDX quote can only be produced inside TDX hardware.
func (*DCAPVerifier) Attest(context.Context, AttestInput) ([]byte, error) {
	return nil, fmt.Errorf("DCAPVerifier is verification-only: a TDX quote can only be produced on TEE hardware (see TDXAttester)")
}

// Verify parses and cryptographically verifies a raw Intel TDX quote. A malformed
// quote is a hard error; a structurally-valid quote that fails signature/chain or
// the measurement policy returns Verified=false (a verdict, not a Go error).
func (d *DCAPVerifier) Verify(_ context.Context, report []byte) (Attestation, error) {
	q, err := abi.QuoteToProto(report)
	if err != nil {
		return Attestation{}, fmt.Errorf("parse tdx quote: %w", err)
	}
	v4, ok := q.(*tdxpb.QuoteV4)
	if !ok {
		return Attestation{}, fmt.Errorf("unsupported tdx quote format (want v4)")
	}

	att := Attestation{Format: "tdx-quote-v4", Signer: "tdx-dcap"}
	if body := v4.GetTdQuoteBody(); body != nil {
		att.Measurement = hex.EncodeToString(body.GetMrTd())
		att.Quote = base64.StdEncoding.EncodeToString(body.GetReportData()) // 64-byte REPORTDATA (our binding nonce)
	}

	now := d.now
	if now.IsZero() {
		now = time.Now()
	}
	opts := &verify.Options{CheckRevocations: d.checkRevocations, GetCollateral: d.getCollateral, Now: now}
	if err := verify.RawTdxQuote(report, opts); err != nil {
		att.Verified = false // genuineness check failed — a verdict, not a transport error
		return att, nil
	}

	if len(d.allowed) > 0 {
		if _, ok := d.allowed[strings.ToLower(att.Measurement)]; !ok {
			att.Verified = false // genuine Intel quote, but the code that ran is not on the approved allowlist
			return att, nil
		}
	}

	att.Verified = true
	return att, nil
}
