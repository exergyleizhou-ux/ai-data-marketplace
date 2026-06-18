package compute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// fakeCertReg records the last Register call (compute.CertRegistrar).
type fakeCertReg struct {
	certID, rtype, rid string
	n                  int
}

func (f *fakeCertReg) Register(_ context.Context, certID, resourceType, resourceID string) error {
	f.certID, f.rtype, f.rid, f.n = certID, resourceType, resourceID, f.n+1
	return nil
}

// blobStore is a storage.Storage whose Open returns a fixed blob; the rest are no-ops.
type blobStore struct{ blob []byte }

func (b blobStore) Open(context.Context, string) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewReader(b.blob)), int64(len(b.blob)), nil
}
func (blobStore) InitMultipart(context.Context, string) (string, error) { return "", nil }
func (blobStore) PutPart(context.Context, string, int, io.Reader) (int64, error) {
	return 0, nil
}
func (blobStore) CompleteMultipart(context.Context, string) (storage.Object, error) {
	return storage.Object{}, nil
}
func (blobStore) Abort(context.Context, string) error  { return nil }
func (blobStore) Delete(context.Context, string) error { return nil }
func (blobStore) Stat(context.Context, string) (storage.UploadStat, error) {
	return storage.UploadStat{}, nil
}

// A released job's certificate must be registered into the public certificates
// table (resource_type=compute_result, resource_id=job_id) under the SAME id the
// buyer-facing certificate uses — so /verify/<cert_id> resolves it.
func TestReleasedJobCertRegisteredForPublicVerify(t *testing.T) {
	blob := []byte("result-bundle-bytes")
	sum := sha256.Sum256(blob)
	want := jobCertificateID("job-1", hex.EncodeToString(sum[:]))

	reg := &fakeCertReg{}
	s := &Service{store: blobStore{blob: blob}, certReg: reg}
	s.registerResultCert(context.Background(), Job{ID: "job-1", Status: JobReleased, OutputKey: "k"})

	if reg.n != 1 {
		t.Fatalf("expected exactly 1 registration, got %d", reg.n)
	}
	if reg.certID != want {
		t.Fatalf("cert id = %q, want %q (must match the buyer-facing cert)", reg.certID, want)
	}
	if reg.rtype != "compute_result" || reg.rid != "job-1" {
		t.Fatalf("resource = %s/%s, want compute_result/job-1", reg.rtype, reg.rid)
	}
}

// Only released jobs register a cert (no leaking a pending/failed job).
func TestUnreleasedJobNotRegistered(t *testing.T) {
	reg := &fakeCertReg{}
	s := &Service{store: blobStore{blob: []byte("x")}, certReg: reg}
	s.registerResultCert(context.Background(), Job{ID: "j", Status: JobQueued, OutputKey: "k"})
	s.registerResultCert(context.Background(), Job{ID: "j", Status: JobReleased, OutputKey: ""})
	if reg.n != 0 {
		t.Fatalf("unreleased / output-less jobs must not register, got %d", reg.n)
	}
}
