package compute

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// fakeData resolves a fixed object key (the mock runner ignores the data).
type fakeData struct{ key string }

func (f fakeData) CurrentObjectKey(context.Context, string) (string, error) { return f.key, nil }

// TestComputeEngineIntegration drives the full execution engine against a real
// Postgres + local object storage: SubmitJob → worker claims → MockRunner runs
// → output gate → output stored → released → buyer reads exact bytes. Then a
// second job whose output exceeds the size cap is rejected (not released).
func TestComputeEngineIntegration(t *testing.T) {
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
	seller := seedUser(t, pool, fmt.Sprintf("eseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("ebuyer-%d", uniq), "buyer")
	dsID := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-eng", Runtime: RuntimeSklearn, Image: "registry/logreg", ImageDigest: "sha256:eng",
		SourceRef: "git:eng", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
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

	// --- happy path: submit → released → exact bytes ---
	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 5})
	job, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	released := waitStatus(t, repo, job.ID, JobReleased, 5*time.Second)
	if released.OutputKey == "" || released.OutputBytes == 0 || released.OutputKind != OutputModel {
		t.Fatalf("released job missing output: %+v", released)
	}

	rc, size, gotJob, err := svc.OpenOutput(ctx, buyer, job.ID)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if int64(len(body)) != size || gotJob.ID != job.ID {
		t.Fatalf("output size mismatch: read=%d size=%d", len(body), size)
	}
	if len(body) == 0 {
		t.Fatal("empty output")
	}

	// A non-owner cannot read the output.
	if _, _, _, err := svc.OpenOutput(ctx, "intruder", job.ID); err == nil {
		t.Fatal("non-owner read should be forbidden")
	}

	// --- output gate: oversize output is rejected, not released ---
	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, MaxOutputBytes: 16, MaxOutputFiles: 4,
	}); err != nil {
		t.Fatalf("shrink offer: %v", err)
	}
	bigJob, err := svc.SubmitJob(ctx, buyer, SubmitInput{
		DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID,
		Params: map[string]any{"_mock_output_bytes": 1000}, // > 16-byte cap
	})
	if err != nil {
		t.Fatalf("submit big: %v", err)
	}
	rej := waitStatus(t, repo, bigJob.ID, JobRejected, 5*time.Second)
	if rej.Error == "" {
		t.Fatalf("rejected job should carry a reason: %+v", rej)
	}
	if rej.OutputKey != "" {
		t.Fatalf("rejected job must not have a downloadable output: %+v", rej)
	}
}

// waitStatus polls a job until it reaches want or the timeout elapses.
func waitStatus(t *testing.T, repo Repository, jobID, want string, timeout time.Duration) Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		j, err := repo.GetJob(context.Background(), jobID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if j.Status == want {
			return j
		}
		if JobTerminal(j.Status) && j.Status != want {
			t.Fatalf("job reached terminal %q, want %q (err=%q)", j.Status, want, j.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for job %s to reach %q (now %q)", jobID, want, j.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
