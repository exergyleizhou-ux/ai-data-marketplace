package compute

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestProcessJob_HonorsOfferSnapshotOverLiveOffer is the audit #6/#7 fix:
// processJob must apply the output-gate config SNAPSHOTTED on the job at submit
// time, not the offer's LIVE config. Otherwise a seller editing the offer after
// a buyer submits could retroactively flip a queued job's review/size behavior
// (config TOCTOU). A nil snapshot (jobs created before migration 000028) falls
// back to the live offer.
func TestProcessJob_HonorsOfferSnapshotOverLiveOffer(t *testing.T) {
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

	seller := seedUser(t, pool, "snapseller", "seller")
	buyer := seedUser(t, pool, "snapbuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-snap", Runtime: RuntimeSklearn, Image: "registry/logreg", ImageDigest: "sha256:snap",
		SourceRef: "git:snap", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 50})

	// MockRunner is enough: we only assert which gate the worker applies.
	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 2, 16))
	defer svc.Close()

	// newJob creates a queued job directly (bypassing SubmitJob's snapshotting) so
	// the test controls the exact snapshot, then drives processJob synchronously.
	newJob := func(t *testing.T, j Job) Job {
		t.Helper()
		j.DatasetID, j.BuyerID, j.EntitlementID = dsID, buyer, ent.ID
		j.AlgorithmID, j.AlgorithmVersion, j.Status = algo.ID, algo.Version, JobQueued
		out, err := repo.CreateJob(ctx, j)
		if err != nil {
			t.Fatalf("create job: %v", err)
		}
		return out
	}
	status := func(t *testing.T, id string) string {
		t.Helper()
		got, err := repo.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		return got.Status
	}
	tru := true
	fls := false

	t.Run("review snapshot wins over live offer", func(t *testing.T) {
		// LIVE offer says do NOT review; the job's SNAPSHOT says review. The
		// snapshot must win → output parked for review, not released.
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, PriceCents: 1000,
			ReviewOutput: false, MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		}); err != nil {
			t.Fatalf("offer: %v", err)
		}
		mb := int64(1 << 20)
		job := newJob(t, Job{ReviewOutput: &tru, MaxOutputBytes: &mb})
		svc.processJob(ctx, job.ID)
		if st := status(t, job.ID); st != JobOutputReviewing {
			t.Fatalf("status = %q, want %q (job snapshot ReviewOutput=true must win over live offer false)", st, JobOutputReviewing)
		}
	})

	t.Run("size snapshot wins over live offer", func(t *testing.T) {
		// LIVE offer permits a large output; the job's SNAPSHOT caps it tiny. A
		// runner output above the snapshot cap (but under the live cap) must be
		// rejected by the snapshot.
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, PriceCents: 1000,
			ReviewOutput: false, MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		}); err != nil {
			t.Fatalf("offer: %v", err)
		}
		mb := int64(100)
		job := newJob(t, Job{
			ReviewOutput:   &fls,
			MaxOutputBytes: &mb,
			Params:         map[string]any{"_mock_output_bytes": 500}, // > snapshot 100, < live 1<<20
		})
		svc.processJob(ctx, job.ID)
		if st := status(t, job.ID); st != JobRejected {
			t.Fatalf("status = %q, want %q (job snapshot MaxOutputBytes=100 must reject 500-byte output)", st, JobRejected)
		}
	})

	t.Run("nil snapshot falls back to live offer", func(t *testing.T) {
		// Pre-migration job: NULL snapshot. The worker must fall back to the live
		// offer — here ReviewOutput=true → parked for review.
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, PriceCents: 1000,
			ReviewOutput: true, MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		}); err != nil {
			t.Fatalf("offer: %v", err)
		}
		job := newJob(t, Job{}) // nil ReviewOutput + nil MaxOutputBytes
		svc.processJob(ctx, job.ID)
		if st := status(t, job.ID); st != JobOutputReviewing {
			t.Fatalf("status = %q, want %q (nil snapshot must fall back to live offer ReviewOutput=true)", st, JobOutputReviewing)
		}
	})
}
