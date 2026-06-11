package compute

import (
	"context"
	"testing"
)

// TestMockKBS_DynamicAllowlist proves the mockKBS allowlist can be updated at
// runtime: Register adds a digest that ReleaseDataKey then accepts; Unregister
// reverts so the next request fails closed with ErrMeasurementNotAllowed.
// This is what the algorithm approval pipeline ultimately uses to keep KBS
// release policy in sync with ops-approved algorithms (Wave 1-6).
func TestMockKBS_DynamicAllowlist(t *testing.T) {
	att := NewMockAttester()
	// Start with a closed allowlist (empty would mean "any verified" — explicit
	// non-empty closed list catches an unregistered digest).
	kbs := NewMockKBS(att, "sha256:zero")
	allowlist, ok := kbs.(MeasurementAllowlist)
	if !ok {
		t.Fatal("mockKBS must implement MeasurementAllowlist")
	}

	digest := "sha256:abc123"
	report, err := att.Attest(context.Background(), AttestInput{
		Measurement: digest, JobID: "j1", OutputSHA: "out",
	})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}

	// Before registration: refused with ErrMeasurementNotAllowed.
	if _, err := kbs.ReleaseDataKey(context.Background(), report, "ds1"); err == nil {
		t.Fatal("ReleaseDataKey for unregistered digest must fail closed")
	}

	allowlist.RegisterMeasurement(digest)

	key, err := kbs.ReleaseDataKey(context.Background(), report, "ds1")
	if err != nil {
		t.Fatalf("ReleaseDataKey after Register: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("released key is empty")
	}

	allowlist.UnregisterMeasurement(digest)
	if _, err := kbs.ReleaseDataKey(context.Background(), report, "ds1"); err == nil {
		t.Fatal("ReleaseDataKey after Unregister must fail closed again")
	}
}
