package compute

import (
	"context"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type recordingCertReg struct{ ids []string }

func (r *recordingCertReg) Register(_ context.Context, certID, _, _ string) error {
	r.ids = append(r.ids, certID)
	return nil
}

// TestScreenAdhoc: the self-serve verification path reuses the runner + gate +
// store + cert machinery without any marketplace job — an uploaded dataset →
// report + a registered, re-hash-verifiable certificate.
func TestScreenAdhoc(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	repo := newFakeRepo()
	if _, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "PaperGuard data-integrity screen", Image: "reg/vo-paperguard",
		ImageDigest: "sha256:pg", OutputKind: OutputMetrics, Status: AlgoApproved, Trusted: true,
	}); err != nil {
		t.Fatalf("register algo: %v", err)
	}
	reg := &recordingCertReg{}
	s := &Service{repo: repo, runner: MockRunner{}, store: store, certReg: reg}

	res, err := s.ScreenAdhoc(ctx, "acct-1", []byte("a,b\n1,2\n3,4\n"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if res.CertID == "" || res.OutputSHA256 == "" || res.OutputBytes == 0 {
		t.Fatalf("incomplete result: %+v", res)
	}
	if res.AlgorithmDigest != "sha256:pg" {
		t.Errorf("digest = %q, want sha256:pg", res.AlgorithmDigest)
	}
	if res.Report == nil {
		t.Error("expected a parsed report")
	}
	if len(reg.ids) != 1 || reg.ids[0] != res.CertID {
		t.Errorf("cert not registered under the result id: %+v vs %s", reg.ids, res.CertID)
	}
}

// TestScreenAdhoc_NoScreener: without a trusted integrity-screen algorithm the
// call fails clearly rather than silently doing nothing.
func TestScreenAdhoc_NoScreener(t *testing.T) {
	store, _ := storage.NewLocal(t.TempDir())
	s := &Service{repo: newFakeRepo(), runner: MockRunner{}, store: store, certReg: &recordingCertReg{}}
	if _, err := s.ScreenAdhoc(context.Background(), "acct-1", []byte("a\n1\n")); err == nil {
		t.Fatal("expected an error when no trusted screener is registered")
	}
}
