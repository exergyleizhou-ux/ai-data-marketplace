package compute

import (
	"reflect"
	"testing"
)

func TestBuildFederatedCertificate_BindsJointProvenance(t *testing.T) {
	fed := FederatedJob{
		ID: "fed-1", BuyerID: "buyer-1", DatasetIDs: []string{"ds-a", "ds-b"},
		Mode: ModeFederated, Status: FedReleased, OutputBytes: 320, UpdatedAt: "2026-06-04T00:00:00Z",
	}
	algo := Algorithm{ID: "algo-1", Name: "fed-logreg", ImageDigest: "sha256:fed", Version: 1, Trusted: true}
	cert := BuildFederatedCertificate(fed, algo, "cafebabe")

	if cert["status"] != "registered" {
		t.Fatalf("status = %v", cert["status"])
	}
	if cert["certificate_id"] != jobCertificateID("fed-1", "cafebabe") {
		t.Fatalf("certificate_id = %v", cert["certificate_id"])
	}
	if cert["federated_job_id"] != "fed-1" || cert["mode"] != ModeFederated {
		t.Fatalf("federated id/mode not bound: %+v", cert)
	}
	if cert["output_sha256"] != "cafebabe" {
		t.Fatalf("output_sha256 = %v", cert["output_sha256"])
	}
	// The participating datasets (parties) must be recorded.
	ids, ok := cert["dataset_ids"].([]string)
	if !ok || !reflect.DeepEqual(ids, []string{"ds-a", "ds-b"}) {
		t.Fatalf("dataset_ids not bound: %+v", cert["dataset_ids"])
	}
	prov, ok := cert["algorithm"].(map[string]any)
	if !ok || prov["image_digest"] != "sha256:fed" {
		t.Fatalf("algorithm provenance wrong: %+v", cert["algorithm"])
	}
}

func TestBuildFederatedCertificate_PSIMode(t *testing.T) {
	fed := FederatedJob{
		ID: "fed-2", DatasetIDs: []string{"ds-a", "ds-b"}, Mode: ModePSI, Status: FedReleased, UpdatedAt: "t",
	}
	cert := BuildFederatedCertificate(fed, Algorithm{Name: "psi-extract"}, "abcd")
	if cert["mode"] != ModePSI {
		t.Fatalf("mode = %v, want psi", cert["mode"])
	}
	if cert["status"] != "registered" {
		t.Fatalf("status = %v", cert["status"])
	}
}

func TestBuildFederatedCertificate_PendingWhenNotReleased(t *testing.T) {
	cert := BuildFederatedCertificate(FederatedJob{ID: "f", Status: FedFanout}, Algorithm{}, "")
	if cert["status"] != "pending" {
		t.Fatalf("status = %v, want pending", cert["status"])
	}
	if _, ok := cert["certificate_id"]; ok {
		t.Fatal("pending certificate must not carry a certificate_id")
	}
}
