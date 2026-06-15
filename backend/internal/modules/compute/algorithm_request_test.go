package compute

import (
	"context"
	"errors"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// fakeAlgoRepo embeds the Repository interface so it satisfies the type while
// implementing only the one method RequestAlgorithm touches. It records what was
// actually handed to the persistence layer.
type fakeAlgoRepo struct {
	Repository
	got Algorithm
}

func (f *fakeAlgoRepo) RegisterAlgorithm(_ context.Context, a Algorithm) (Algorithm, error) {
	if a.Status == "" {
		a.Status = AlgoPending
	}
	a.ID = "algo-1"
	f.got = a
	return a, nil
}

type noopRecorder struct{}

func (noopRecorder) Record(context.Context, audit.Entry) {}

// TestRequestAlgorithmForcesPendingUntrustedOwned is the security contract for
// buyer-submitted algorithms: no matter what the caller claims, a request is
// stored as pending (never runnable — the run-gate requires AlgoApproved),
// untrusted, and attributed to the requester. A buyer cannot self-approve,
// self-trust, or spoof ownership.
func TestRequestAlgorithmForcesPendingUntrustedOwned(t *testing.T) {
	repo := &fakeAlgoRepo{}
	svc := &Service{repo: repo, audit: noopRecorder{}}

	out, err := svc.RequestAlgorithm(context.Background(), "buyer-42", Algorithm{
		Name:       "my-clustering",
		Runtime:    "python-sklearn",
		Image:      "ghcr.io/example/clustering",
		OutputKind: OutputModel,
		Trusted:    true,         // attempt to self-declare trust
		Status:     AlgoApproved, // attempt to self-approve
		OwnerID:    "victim-seller",
	})
	if err != nil {
		t.Fatalf("RequestAlgorithm: %v", err)
	}
	// Returned object must be neutered.
	if out.Status != AlgoPending {
		t.Errorf("status = %q, want pending", out.Status)
	}
	if out.Trusted {
		t.Error("trusted must be forced false for a buyer request")
	}
	if out.OwnerID != "buyer-42" {
		t.Errorf("owner = %q, want buyer-42", out.OwnerID)
	}
	// And the values actually persisted must be neutered too (defence in depth).
	if repo.got.Status != AlgoPending || repo.got.Trusted || repo.got.OwnerID != "buyer-42" {
		t.Errorf("persisted = %+v, want pending/untrusted/owned-by-requester", repo.got)
	}
}

func TestRequestAlgorithmRequiresRequester(t *testing.T) {
	svc := &Service{repo: &fakeAlgoRepo{}, audit: noopRecorder{}}
	_, err := svc.RequestAlgorithm(context.Background(), "", Algorithm{
		Name: "x", Runtime: "python-sklearn", Image: "img", OutputKind: OutputModel,
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("empty requester must be ErrValidation, got %v", err)
	}
}
