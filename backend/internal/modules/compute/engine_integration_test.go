package compute

import (
	"context"
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

	seller := seedUser(t, pool, "eseller", "seller")
	buyer := seedUser(t, pool, "ebuyer", "buyer")
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
	// §21: a rejected job refunds the credit. Only the first (released) job
	// should have consumed quota.
	afterRej, _ := repo.GetEntitlement(ctx, ent.ID)
	if afterRej.JobsUsed != 1 {
		t.Fatalf("rejected job did not refund credit: jobs_used=%d, want 1", afterRej.JobsUsed)
	}

	// --- review_output: output parks in output_reviewing until ops releases ---
	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, MaxOutputBytes: 1 << 20, ReviewOutput: true,
	}); err != nil {
		t.Fatalf("review offer: %v", err)
	}
	revJob, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit review job: %v", err)
	}
	reviewing := waitStatus(t, repo, revJob.ID, JobOutputReviewing, 5*time.Second)
	if reviewing.OutputKey == "" {
		t.Fatalf("reviewing job should have its output staged: %+v", reviewing)
	}
	// Buyer cannot download while it is under review.
	if _, _, _, err := svc.OpenOutput(ctx, buyer, revJob.ID); err == nil {
		t.Fatal("output under review must not be downloadable")
	}
	// Ops releases it → now downloadable.
	if _, err := svc.OpsReleaseOutput(ctx, "ops-1", revJob.ID); err != nil {
		t.Fatalf("ops release: %v", err)
	}
	relJob, _ := repo.GetJob(ctx, revJob.ID)
	if relJob.Status != JobReleased {
		t.Fatalf("ops-released job status=%s, want released", relJob.Status)
	}
	rc2, _, _, err := svc.OpenOutput(ctx, buyer, revJob.ID)
	if err != nil {
		t.Fatalf("download after ops release: %v", err)
	}
	rc2.Close()

	// And an ops reject withholds the output + refunds the credit.
	usedBefore, _ := repo.GetEntitlement(ctx, ent.ID)
	rev2, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit review job 2: %v", err)
	}
	waitStatus(t, repo, rev2.ID, JobOutputReviewing, 5*time.Second)
	if _, err := svc.OpsRejectOutput(ctx, "ops-1", rev2.ID, "looks like raw data"); err != nil {
		t.Fatalf("ops reject: %v", err)
	}
	rejected2, _ := repo.GetJob(ctx, rev2.ID)
	if rejected2.Status != JobRejected {
		t.Fatalf("ops-rejected job status=%s, want rejected", rejected2.Status)
	}
	usedAfter, _ := repo.GetEntitlement(ctx, ent.ID)
	if usedAfter.JobsUsed != usedBefore.JobsUsed {
		t.Fatalf("ops reject did not refund: before=%d after=%d", usedBefore.JobsUsed, usedAfter.JobsUsed)
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
