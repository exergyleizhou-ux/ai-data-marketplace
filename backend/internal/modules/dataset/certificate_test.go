package dataset

import (
	"context"
	"strings"
	"testing"
)

func TestBuildCertificateRegistered(t *testing.T) {
	d := Dataset{ID: "ds-7", Title: "T", CreatedAt: "2026-06-02T00:00:00Z"}
	vm := VersionMeta{VersionNo: 2, ContentSHA256: "abc123"}
	checks := []QualityCheck{
		{Type: "authenticity", Report: map[string]any{"applicable": true, "band": "clean"}},
		{Type: "pii_redaction", Report: map[string]any{"verified": true}},
	}
	cert := BuildCertificate(d, vm, checks)

	if cert["status"] != "registered" {
		t.Fatalf("status = %v", cert["status"])
	}
	id, _ := cert["certificate_id"].(string)
	if !strings.HasPrefix(id, "VO-") || len(id) != 15 {
		t.Errorf("certificate_id = %q, want VO-<12 hex>", id)
	}
	// Deterministic: same inputs -> same code.
	if id != certificateID("ds-7", "abc123") {
		t.Errorf("certificate id not deterministic")
	}
	if cert["content_sha256"] != "abc123" || cert["registered_at"] != "2026-06-02T00:00:00Z" {
		t.Errorf("integrity fields = %v / %v", cert["content_sha256"], cert["registered_at"])
	}
	q := cert["quality"].(map[string]any)
	if q["authenticity_band"] != "clean" || q["pii_deidentified"] != "verified-zero-residual" {
		t.Errorf("quality = %v", q)
	}
}

func TestBuildCertificatePending(t *testing.T) {
	cert := BuildCertificate(Dataset{ID: "ds-x"}, VersionMeta{}, nil)
	if cert["status"] != "pending" {
		t.Errorf("no content -> pending, got %v", cert["status"])
	}
	if _, ok := cert["certificate_id"]; ok {
		t.Errorf("pending cert must not have an id")
	}
}

func TestCertificateService(t *testing.T) {
	repo := newFakeRepo()
	repo.items["ds-1"] = Dataset{ID: "ds-1", Title: "T", CreatedAt: "2026-06-02T00:00:00Z"}
	svc := NewService(repo, fakeIdentity{status: map[string]string{}}, nil)

	cert, err := svc.Certificate(context.Background(), "ds-1")
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	// fakeRepo.CurrentVersionMeta returns ContentSHA256 "deadbeef" -> registered.
	if cert["status"] != "registered" {
		t.Errorf("status = %v", cert["status"])
	}
	if _, err := svc.Certificate(context.Background(), "missing"); err == nil {
		t.Errorf("unknown dataset should error")
	}
}
