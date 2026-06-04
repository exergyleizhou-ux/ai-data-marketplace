package compute

import (
	"regexp"
	"testing"
)

func TestJobCertificateID_Deterministic(t *testing.T) {
	a := jobCertificateID("job-7", "abc123")
	b := jobCertificateID("job-7", "abc123")
	if a != b {
		t.Fatalf("certificate id not deterministic: %q vs %q", a, b)
	}
	if !regexp.MustCompile(`^VO-[0-9A-F]{12}$`).MatchString(a) {
		t.Fatalf("certificate_id = %q, want VO-<12 hex>", a)
	}
	if jobCertificateID("job-7", "abc123") == jobCertificateID("job-8", "abc123") {
		t.Fatal("different jobs must yield different certificate ids")
	}
}

func TestBuildJobCertificate_BindsProvenance(t *testing.T) {
	job := Job{
		ID: "job-9", DatasetID: "ds-1", BuyerID: "buyer-1",
		Status: JobReleased, OutputKind: OutputModel, OutputBytes: 465, FinishedAt: "2026-06-04T00:00:00Z",
	}
	algo := Algorithm{ID: "algo-1", Name: "logreg", ImageDigest: "sha256:codedigest", Version: 2, Trusted: true}
	cert := BuildJobCertificate(job, algo, "deadbeefcafe")

	if cert["status"] != "registered" {
		t.Fatalf("status = %v", cert["status"])
	}
	if cert["certificate_id"] != jobCertificateID("job-9", "deadbeefcafe") {
		t.Fatalf("certificate_id = %v", cert["certificate_id"])
	}
	if cert["job_id"] != "job-9" || cert["dataset_id"] != "ds-1" {
		t.Fatalf("job/dataset not bound: %+v", cert)
	}
	if cert["output_sha256"] != "deadbeefcafe" {
		t.Fatalf("output_sha256 = %v", cert["output_sha256"])
	}
	// The audited code must be bound: algorithm + its pinned image digest.
	prov, ok := cert["algorithm"].(map[string]any)
	if !ok {
		t.Fatalf("algorithm provenance missing: %+v", cert)
	}
	if prov["image_digest"] != "sha256:codedigest" || prov["name"] != "logreg" || prov["trusted"] != true {
		t.Fatalf("algorithm provenance wrong: %+v", prov)
	}
}

func TestBuildJobCertificate_PendingWhenNoOutput(t *testing.T) {
	cert := BuildJobCertificate(Job{ID: "j", Status: JobRunning}, Algorithm{}, "")
	if cert["status"] != "pending" {
		t.Fatalf("a job with no released output must be pending, got %v", cert["status"])
	}
	if _, ok := cert["certificate_id"]; ok {
		t.Fatal("pending certificate must not carry a certificate_id")
	}
}
