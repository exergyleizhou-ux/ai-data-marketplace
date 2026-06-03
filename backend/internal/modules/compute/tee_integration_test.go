package compute

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestComputeTEEAttestationIntegration drives an L2 job through the TEE runner
// (MockRunner wrapped with attestation) against a real Postgres: the job is
// submitted, runs, the attestation is stored on the job, and the buyer/seller
// can fetch a server-verified report bound to the algorithm digest. A real TEE
// runner swaps the base + a hardware attester (design P3); the plumbing is the
// same and is exercised here without TEE hardware.
func TestComputeTEEAttestationIntegration(t *testing.T) {
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
	seller := seedUser(t, pool, fmt.Sprintf("teeseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("teebuyer-%d", uniq), "buyer")
	dsID := seedDataset(t, pool, seller)

	const digest = "sha256:codedigest-l2"
	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-l2", Runtime: RuntimeSklearn, Image: "registry/logreg", ImageDigest: digest,
		SourceRef: "git:l2", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL2, PriceCents: 1000, MaxOutputBytes: 1 << 20,
	}); err != nil {
		t.Fatalf("offer: %v", err)
	}

	att := NewMockAttester()
	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewTEERunner(NewMockRunner(), att), store, fakeData{key: "datasets/x"}, 2, 8),
		WithAttester(att))
	defer svc.Close()

	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 1})
	job, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	released := waitStatus(t, repo, job.ID, JobReleased, 5*time.Second)
	if len(released.Attestation) == 0 {
		t.Fatalf("L2 released job must carry an attestation: %+v", released)
	}

	// Buyer fetches a server-verified attestation bound to the algorithm digest.
	a, err := svc.GetAttestation(ctx, buyer, job.ID)
	if err != nil {
		t.Fatalf("buyer get attestation: %v", err)
	}
	if !a.Verified {
		t.Fatal("stored attestation should verify server-side")
	}
	if a.Measurement != digest || a.OutputSHA == "" {
		t.Fatalf("attestation not bound to the code/output: %+v", a)
	}

	// The dataset's seller may also view it; an unrelated user may not.
	if _, err := svc.GetAttestation(ctx, seller, job.ID); err != nil {
		t.Fatalf("seller get attestation: %v", err)
	}
	if _, err := svc.GetAttestation(ctx, "intruder", job.ID); err == nil {
		t.Fatal("unrelated user must not read the attestation")
	}
}
