package compute

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestProcessJob_L2OfferRefusedByNonTEERunner is the security invariant tax:
// an offer with TrustLevel=L2 must NEVER run on a non-TEE runner. The runner
// is selected globally at startup (server.go:436-462), so if an operator
// deploys with COMPUTE_RUNNER=mock|docker but a seller publishes an L2 offer,
// the worker must fail-closed rather than silently releasing output without
// attestation. Pre-fix, worker.processJob would happily call s.runner.Run on
// MockRunner and the only attestation check (worker.go:226) only STORES the
// report if non-empty — it never FAILS the job for missing it on an L2 offer.
func TestProcessJob_L2OfferRefusedByNonTEERunner(t *testing.T) {
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

	seller := seedUser(t, pool, "tseller", "seller")
	buyer := seedUser(t, pool, "tbuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-trust", Runtime: RuntimeSklearn, Image: "registry/logreg", ImageDigest: "sha256:trust",
		SourceRef: "git:trust", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	if _, err := repo.UpsertOffer(ctx, Offer{
		// THE TEST: trust level L2 with a non-TEE runner below.
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL2, PriceCents: 1000,
		MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
	}); err != nil {
		t.Fatalf("offer: %v", err)
	}

	// MockRunner.Kind() = "mock", not "tee:*" — must be rejected for L2.
	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 2, 16))
	defer svc.Close()

	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 5})
	job, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Poll briefly for terminal state.
	var terminal Job
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := repo.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if j.Status == JobFailed || j.Status == JobReleased || j.Status == JobRejected {
			terminal = j
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if terminal.Status != JobFailed {
		t.Fatalf("L2 offer with mock runner: status = %q, want %q (refused before run)", terminal.Status, JobFailed)
	}
	if terminal.Error != "trust_l2_runner_mismatch" {
		t.Errorf("L2 offer with mock runner: error = %q, want trust_l2_runner_mismatch", terminal.Error)
	}
}
