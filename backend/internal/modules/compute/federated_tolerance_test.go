package compute

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// fedToleranceSetup spins real PG + storage + a fed-logreg service, seeds a
// seller/buyer + N datasets (caps[i] = MaxOutputBytes for dataset i; a tiny cap
// forces that sub-job to be gate-rejected), grants entitlements, and returns the
// service + dataset ids + entitlement ids.
func fedToleranceSetup(t *testing.T, prefix string, caps []int64) (svc *Service, repo Repository, buyer, algoID string, dsIDs, entIDs []string) {
	t.Helper()
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
	t.Cleanup(pool.Close)
	ctx := context.Background()
	repo = NewRepository(pool)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	seller := seedUser(t, pool, prefix+"seller", "seller")
	buyer = seedUser(t, pool, prefix+"buyer", "buyer")

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "fed-logreg", Runtime: RuntimeFedLogreg, Image: "registry/fedlogreg", ImageDigest: "sha256:fed",
		SourceRef: "git:fed", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	algoID = algo.ID

	for i, capBytes := range caps {
		ds := seedDataset(t, pool, seller)
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: ds, Enabled: true, AllowFederated: true, TrustLevel: TrustL1,
			MaxOutputBytes: capBytes, MaxOutputFiles: 4,
		}); err != nil {
			t.Fatalf("offer %d: %v", i, err)
		}
		ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds, BuyerID: buyer, JobsQuota: 3})
		dsIDs = append(dsIDs, ds)
		entIDs = append(entIDs, ent.ID)
	}

	svc = NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 3, 16))
	t.Cleanup(svc.Close)
	return svc, repo, buyer, algoID, dsIDs, entIDs
}

// TestComputeFederatedToleranceSurvivors: 3 datasets, ds3 has a tiny output cap
// so its sub-job is gate-rejected. With min_participants=2 the job still succeeds
// on the 2 survivors; the dropout is refunded, the survivors are billed.
func TestComputeFederatedToleranceSurvivors(t *testing.T) {
	const big = 1 << 20
	svc, repo, buyer, algoID, ds, ent := fedToleranceSetup(t, "ftol", []int64{big, big, 10}) // ds3 cap=10 → reject
	ctx := context.Background()

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algoID, DatasetIDs: ds, MinParticipants: 2,
	})
	if err != nil {
		t.Fatalf("submit federated: %v", err)
	}
	released := waitFedStatus(t, repo, fed.ID, FedReleased, 10*time.Second)

	rc, _, _, err := svc.OpenFederatedOutput(ctx, buyer, released.ID)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	var joint struct {
		Participants int `json:"participants"`
	}
	_ = json.Unmarshal(body, &joint)
	if joint.Participants != 2 {
		t.Fatalf("joint participants=%d, want 2 (one dropout)", joint.Participants)
	}

	// ds1, ds2 billed (1 each); ds3 (dropout) refunded (0).
	e1, _ := repo.GetEntitlement(ctx, ent[0])
	e2, _ := repo.GetEntitlement(ctx, ent[1])
	e3, _ := repo.GetEntitlement(ctx, ent[2])
	if e1.JobsUsed != 1 || e2.JobsUsed != 1 {
		t.Fatalf("survivors should be billed: e1=%d e2=%d, want 1/1", e1.JobsUsed, e2.JobsUsed)
	}
	if e3.JobsUsed != 0 {
		t.Fatalf("dropout should be refunded: e3=%d, want 0", e3.JobsUsed)
	}
}

// TestComputeFederatedBelowThreshold: 2 of 3 datasets fail (tiny caps); with
// min_participants=2 only 1 survives (< 2) → whole job fails, all 3 refunded.
func TestComputeFederatedBelowThreshold(t *testing.T) {
	const big = 1 << 20
	svc, repo, buyer, algoID, ds, ent := fedToleranceSetup(t, "fbelow", []int64{big, 10, 10}) // ds2,ds3 reject
	ctx := context.Background()

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algoID, DatasetIDs: ds, MinParticipants: 2,
	})
	if err != nil {
		t.Fatalf("submit federated: %v", err)
	}
	failed := waitFedStatus(t, repo, fed.ID, FedFailed, 10*time.Second)
	if failed.FailureCode != "insufficient_participants" {
		t.Fatalf("failure code=%q, want insufficient_participants", failed.FailureCode)
	}
	for i, eid := range ent {
		e, _ := repo.GetEntitlement(ctx, eid)
		if e.JobsUsed != 0 {
			t.Fatalf("entitlement %d not refunded: jobs_used=%d, want 0", i, e.JobsUsed)
		}
	}
}

// TestComputeFederatedMinParticipantsValidation rejects out-of-range min.
func TestComputeFederatedMinParticipantsValidation(t *testing.T) {
	svc, _, buyer, algoID, ds, _ := fedToleranceSetup(t, "fval", []int64{1 << 20, 1 << 20})
	ctx := context.Background()
	if _, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algoID, DatasetIDs: ds, MinParticipants: 3, // > N=2
	}); err == nil {
		t.Fatal("want validation error for min_participants > N")
	}
}
