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

// TestComputeOutputGate_StructuralRejection drives the full engine and asserts
// that an algorithm whose output is WITHIN the size cap but NOT a structured
// aggregate (a raw blob — the steganographic-exfil shape the size gate alone
// misses) is rejected, not released, and the buyer's credit is refunded.
func TestComputeOutputGate_StructuralRejection(t *testing.T) {
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

	seller := seedUser(t, pool, "gateseller", "seller")
	buyer := seedUser(t, pool, "gatebuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "raw-exfil", Runtime: RuntimeSklearn, Image: "registry/raw", ImageDigest: "sha256:raw",
		SourceRef: "git:raw", OutputKind: OutputAggregate, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	// A generous size cap (1 MiB): the raw blob is well under it, so ONLY the
	// structural gate can catch it.
	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, PriceCents: 1000,
		MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
	}); err != nil {
		t.Fatalf("offer: %v", err)
	}

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 2, 16))
	defer svc.Close()

	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 5})
	job, err := svc.SubmitJob(ctx, buyer, SubmitInput{
		DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID,
		Params: map[string]any{"_mock_output_raw": "id,ssn,balance\n1,123-45-6789,9999\n2,222-33-4444,1\n"},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	rej := waitStatus(t, repo, job.ID, JobRejected, 5*time.Second)
	if rej.Error != ReasonNotStructured {
		t.Fatalf("expected reject reason %q, got %q", ReasonNotStructured, rej.Error)
	}
	if rej.OutputKey != "" {
		t.Fatalf("rejected job must not have a downloadable output: %+v", rej)
	}
	// §21: a structurally-rejected job refunds the credit (no quota consumed).
	after, _ := repo.GetEntitlement(ctx, ent.ID)
	if after.JobsUsed != 0 {
		t.Fatalf("rejected job did not refund credit: jobs_used=%d, want 0", after.JobsUsed)
	}
}
