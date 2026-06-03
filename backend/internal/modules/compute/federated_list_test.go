package compute

import (
	"context"
	"testing"
)

// TestComputeFederatedListByBuyer: a submitted federated job shows up in the
// buyer's list and is scoped to that buyer.
func TestComputeFederatedListByBuyer(t *testing.T) {
	svc, repo, buyer, algoID, ds, _ := fedToleranceSetup(t, "flist", []int64{1 << 20, 1 << 20})
	ctx := context.Background()

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{AlgorithmID: algoID, DatasetIDs: ds})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	items, err := svc.ListFederatedJobs(ctx, buyer, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, f := range items {
		if f.ID == fed.ID {
			found = true
			if f.BuyerID != buyer {
				t.Fatalf("buyer mismatch: %s", f.BuyerID)
			}
			if len(f.DatasetIDs) != 2 {
				t.Fatalf("dataset_ids=%d want 2", len(f.DatasetIDs))
			}
		}
	}
	if !found {
		t.Fatalf("submitted federated job %s not in buyer's list (%d items)", fed.ID, len(items))
	}

	// Scoped to buyer: a different buyer id sees none of it.
	_ = repo
	otherItems, err := svc.ListFederatedJobs(ctx, "00000000-0000-0000-0000-0000000000aa", 50, 0)
	if err != nil {
		t.Fatalf("list other: %v", err)
	}
	for _, f := range otherItems {
		if f.ID == fed.ID {
			t.Fatal("federated job leaked to another buyer's list")
		}
	}
}
