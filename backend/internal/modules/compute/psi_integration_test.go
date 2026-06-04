package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestComputePSIIntegration drives the full Direction-D PSI loop against a real
// Postgres + local storage: a federated job whose algorithm runtime is
// psi-extract fans out one sub-job per dataset → each MockRunner emits its party
// set (psi-set-v1) → event-driven aggregation runs the PSI orchestrator → the
// intersection is released as the joint output. Asserts: the job releases a
// psi-result-v1 with the shared elements, sub-job party sets are NOT
// buyer-downloadable, and each entitlement is spent once.
func TestComputePSIIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB integration test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	repo := NewRepository(pool)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	uniq := time.Now().UnixNano()
	seller := seedUser(t, pool, fmt.Sprintf("pseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("pbuyer-%d", uniq), "buyer")
	ds1 := seedDataset(t, pool, seller)
	ds2 := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "psi-extract", Runtime: RuntimePSIExtract, Image: "registry/psi", ImageDigest: "sha256:psi",
		SourceRef: "git:psi", OutputKind: OutputAggregate, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	for _, ds := range []string{ds1, ds2} {
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: ds, Enabled: true, AllowFederated: true, TrustLevel: TrustL1,
			MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		}); err != nil {
			t.Fatalf("offer %s: %v", ds, err)
		}
	}

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 2, 16))
	defer svc.Close()

	if _, err := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds1, BuyerID: buyer, JobsQuota: 3}); err != nil {
		t.Fatalf("ent1: %v", err)
	}
	if _, err := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds2, BuyerID: buyer, JobsQuota: 3}); err != nil {
		t.Fatalf("ent2: %v", err)
	}

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: []string{ds1, ds2},
	})
	if err != nil {
		t.Fatalf("submit psi: %v", err)
	}
	if fed.Mode != ModePSI {
		t.Fatalf("psi job mode = %q, want psi", fed.Mode)
	}

	released := waitFedStatus(t, repo, fed.ID, FedReleased, 10*time.Second)
	if released.OutputKey == "" || released.OutputBytes == 0 {
		t.Fatalf("released psi job missing joint output: %+v", released)
	}

	rc, _, _, err := svc.OpenFederatedOutput(ctx, buyer, fed.ID)
	if err != nil {
		t.Fatalf("open psi output: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	var res struct {
		Format       string   `json:"_format"`
		Intersection []string `json:"intersection"`
		Cardinality  int      `json:"cardinality"`
		Participants int      `json:"participants"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("unmarshal psi result: %v", err)
	}
	if res.Format != "psi-result-v1" || res.Participants != 2 {
		t.Fatalf("bad psi result: %+v", res)
	}
	// Both datasets share {shared-a, shared-b}; their dataset-specific elements differ.
	if res.Cardinality != 2 {
		t.Fatalf("intersection cardinality = %d, want 2 (the shared elements); got %v", res.Cardinality, res.Intersection)
	}
	for _, want := range []string{"shared-a", "shared-b"} {
		found := false
		for _, e := range res.Intersection {
			if e == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("intersection %v missing %q", res.Intersection, want)
		}
	}

	// Sub-job party sets must NOT be buyer-downloadable (only the joint result is).
	subs, err := repo.ListSubJobs(ctx, fed.ID)
	if err != nil || len(subs) != 2 {
		t.Fatalf("list sub-jobs: %v (n=%d)", err, len(subs))
	}
	for _, sj := range subs {
		if sj.Status != JobReleased {
			t.Fatalf("sub-job %s status=%q, want released", sj.ID, sj.Status)
		}
		if _, _, _, derr := svc.OpenOutput(ctx, buyer, sj.ID); derr == nil {
			t.Fatal("sub-job party set must not be buyer-downloadable")
		}
	}

	// Each dataset's entitlement spent exactly once.
	ents, err := repo.ListEntitlementsByBuyer(ctx, buyer, 100, 0)
	if err != nil {
		t.Fatalf("list ents: %v", err)
	}
	for _, e := range ents {
		if e.JobsUsed != 1 {
			t.Fatalf("entitlement for %s used=%d, want 1", e.DatasetID, e.JobsUsed)
		}
	}
}
