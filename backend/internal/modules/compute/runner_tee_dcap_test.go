package compute

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/google/go-tdx-guest/testing/testdata"
)

// The bundled sample is a real Intel TDX production quote (SPR); its PCK certs
// are valid around mid-2023, so verification is pinned to a time in that window.
var sampleQuoteTime = time.Date(2023, time.July, 1, 1, 0, 0, 0, time.UTC)

// TestDCAPVerifier_RealSampleQuoteVerifies: a genuine Intel-signed TDX quote
// passes offline verification (ECDSA signature + PCK chain to Intel's root),
// and the verifier extracts the MRTD measurement.
func TestDCAPVerifier_RealSampleQuoteVerifies(t *testing.T) {
	v := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime})
	att, err := v.Verify(context.Background(), testdata.RawQuote)
	if err != nil {
		t.Fatalf("verify real Intel sample quote: %v", err)
	}
	if !att.Verified {
		t.Fatal("a real Intel production TDX quote must verify offline (sig + PCK chain)")
	}
	if att.Signer != "tdx-dcap" {
		t.Errorf("signer = %q, want tdx-dcap", att.Signer)
	}
	if att.Measurement == "" {
		t.Fatal("expected a non-empty MRTD measurement")
	}
	if _, err := hex.DecodeString(att.Measurement); err != nil {
		t.Errorf("measurement should be hex: %v", err)
	}
}

// TestDCAPVerifier_TamperedQuoteDoesNotVerify: flipping a byte in the TD report
// body breaks the ECDSA signature → not verified (never a false positive).
func TestDCAPVerifier_TamperedQuoteDoesNotVerify(t *testing.T) {
	tampered := append([]byte(nil), testdata.RawQuote...)
	tampered[120] ^= 0xff // inside the TD report body (past the 48-byte header)
	v := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime})
	att, err := v.Verify(context.Background(), tampered)
	if err == nil && att.Verified {
		t.Fatal("a tampered quote must NOT verify")
	}
}

// TestDCAPVerifier_AllowlistRejectsUnknownMeasurement: an authentic quote whose
// MRTD is not in the approved-algorithm allowlist is rejected (policy gate).
func TestDCAPVerifier_AllowlistRejectsUnknownMeasurement(t *testing.T) {
	v := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime, AllowedMeasurements: []string{strings.Repeat("00", 48)}})
	att, _ := v.Verify(context.Background(), testdata.RawQuote)
	if att.Verified {
		t.Fatal("an authentic quote with an unapproved MRTD must not verify")
	}
}

// TestDCAPVerifier_AllowlistAcceptsKnownMeasurement: allowlisting the sample's
// own MRTD lets the authentic quote through.
func TestDCAPVerifier_AllowlistAcceptsKnownMeasurement(t *testing.T) {
	base, err := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime}).Verify(context.Background(), testdata.RawQuote)
	if err != nil || !base.Verified {
		t.Fatalf("baseline verify failed: %v %+v", err, base)
	}
	v := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime, AllowedMeasurements: []string{base.Measurement}})
	att, err := v.Verify(context.Background(), testdata.RawQuote)
	if err != nil || !att.Verified {
		t.Fatalf("authentic quote with allowlisted MRTD must verify: %v %+v", err, att)
	}
}

// TestDCAPVerifier_AttestUnsupported: a verifier cannot mint a quote (that needs
// TDX hardware); Attest must error rather than fabricate one.
func TestDCAPVerifier_AttestUnsupported(t *testing.T) {
	if _, err := NewDCAPVerifier(DCAPConfig{}).Attest(context.Background(), AttestInput{Measurement: "sha256:x"}); err == nil {
		t.Fatal("DCAP verifier is verification-only; Attest must error")
	}
}

// TestDCAPVerifier_MalformedQuoteErrors: garbage input is a hard parse error,
// not a silent "unverified".
func TestDCAPVerifier_MalformedQuoteErrors(t *testing.T) {
	att, err := NewDCAPVerifier(DCAPConfig{Now: sampleQuoteTime}).Verify(context.Background(), []byte("not a quote"))
	if err == nil && att.Verified {
		t.Fatal("garbage bytes must not verify")
	}
}
