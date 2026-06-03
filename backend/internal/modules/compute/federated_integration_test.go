package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestComputeFederatedIntegration drives the full P4-a loop against a real
// Postgres + local storage: SubmitFederatedJob fans out N sandbox sub-jobs →
// each MockRunner emits fedparams-v1 → event-driven aggregation runs real FedAvg
// → joint model released. Asserts: joint weights equal FedAvg of the partials,
// sub-job partials are NOT buyer-downloadable, and each dataset's entitlement
// is spent exactly once.
func TestComputeFederatedIntegration(t *testing.T) {
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
	seller := seedUser(t, pool, fmt.Sprintf("fseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("fbuyer-%d", uniq), "buyer")
	ds1 := seedDataset(t, pool, seller)
	ds2 := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "fed-logreg", Runtime: RuntimeFedLogreg, Image: "registry/fedlogreg", ImageDigest: "sha256:fed",
		SourceRef: "git:fed", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
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

	ent1, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds1, BuyerID: buyer, JobsQuota: 3})
	ent2, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds2, BuyerID: buyer, JobsQuota: 3})

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: []string{ds1, ds2},
	})
	if err != nil {
		t.Fatalf("submit federated: %v", err)
	}
	if fed.Status != FedFanout {
		t.Fatalf("federated status after submit=%q, want fanout", fed.Status)
	}

	released := waitFedStatus(t, repo, fed.ID, FedReleased, 10*time.Second)
	if released.OutputKey == "" || released.OutputBytes == 0 {
		t.Fatalf("released federated job missing joint output: %+v", released)
	}

	// Joint output is a fedmodel-v1 = real FedAvg of the two sub-job partials.
	rc, _, gotFed, err := svc.OpenFederatedOutput(ctx, buyer, fed.ID)
	if err != nil {
		t.Fatalf("open federated output: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if gotFed.ID != fed.ID {
		t.Fatalf("output federated id mismatch")
	}
	var joint struct {
		Format       string    `json:"_format"`
		Weights      []float64 `json:"weights"`
		Intercept    float64   `json:"intercept"`
		NTotal       int       `json:"n_total"`
		Participants int       `json:"participants"`
	}
	if err := json.Unmarshal(body, &joint); err != nil {
		t.Fatalf("unmarshal joint: %v", err)
	}
	if joint.Format != "fedmodel-v1" || joint.Participants != 2 || joint.NTotal <= 0 {
		t.Fatalf("bad joint model: %+v", joint)
	}

	// Re-derive the expected FedAvg from the stored sub-job partials and compare.
	subs, err := repo.ListSubJobs(ctx, fed.ID)
	if err != nil || len(subs) != 2 {
		t.Fatalf("list sub-jobs: %v (n=%d)", err, len(subs))
	}
	var partials []Partial
	for _, sj := range subs {
		if sj.Status != JobReleased {
			t.Fatalf("sub-job %s status=%q, want released", sj.ID, sj.Status)
		}
		src, _, oerr := store.Open(ctx, sj.OutputKey)
		if oerr != nil {
			t.Fatalf("open sub output: %v", oerr)
		}
		raw, _ := io.ReadAll(src)
		src.Close()
		p, perr := parsePartial(raw)
		if perr != nil {
			t.Fatalf("parse sub partial: %v", perr)
		}
		partials = append(partials, p)
		// Buyer must NOT be able to download or directly fetch a sub-job partial.
		if _, _, _, derr := svc.OpenOutput(ctx, buyer, sj.ID); derr == nil {
			t.Fatal("sub-job partial must not be buyer-downloadable")
		}
		if _, gerr := svc.GetJob(ctx, buyer, sj.ID); gerr == nil {
			t.Fatal("sub-job must not be directly gettable by buyer")
		}
	}
	expRaw, _ := FedAvgAggregator{}.Aggregate(partials)
	var exp struct {
		Weights   []float64 `json:"weights"`
		Intercept float64   `json:"intercept"`
	}
	_ = json.Unmarshal(expRaw, &exp)
	if len(joint.Weights) != len(exp.Weights) {
		t.Fatalf("weight dim mismatch: got %d want %d", len(joint.Weights), len(exp.Weights))
	}
	for i := range exp.Weights {
		if math.Abs(joint.Weights[i]-exp.Weights[i]) > 1e-9 {
			t.Fatalf("joint weight[%d]=%v != FedAvg %v", i, joint.Weights[i], exp.Weights[i])
		}
	}
	if math.Abs(joint.Intercept-exp.Intercept) > 1e-9 {
		t.Fatalf("joint intercept=%v != FedAvg %v", joint.Intercept, exp.Intercept)
	}

	// Each dataset's entitlement spent exactly once.
	e1, _ := repo.GetEntitlement(ctx, ent1.ID)
	e2, _ := repo.GetEntitlement(ctx, ent2.ID)
	if e1.JobsUsed != 1 || e2.JobsUsed != 1 {
		t.Fatalf("entitlement spend: e1=%d e2=%d, want 1/1", e1.JobsUsed, e2.JobsUsed)
	}
}

// TestComputeFederatedFailureRefund verifies that when one sub-job fails the
// whole federated job fails and ALL participants' credits are refunded (spec §9).
func TestComputeFederatedFailureRefund(t *testing.T) {
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
	seller := seedUser(t, pool, fmt.Sprintf("ffseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("ffbuyer-%d", uniq), "buyer")
	ds1 := seedDataset(t, pool, seller)
	ds2 := seedDataset(t, pool, seller)

	algo, _ := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "fed-logreg", Runtime: RuntimeFedLogreg, Image: "registry/fedlogreg", ImageDigest: "sha256:fed",
		SourceRef: "git:fed", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	// ds1 OK; ds2 has a tiny output cap so its ~60-byte fedparams output is gate-rejected.
	if _, err := repo.UpsertOffer(ctx, Offer{DatasetID: ds1, Enabled: true, AllowFederated: true,
		TrustLevel: TrustL1, MaxOutputBytes: 1 << 20, MaxOutputFiles: 4}); err != nil {
		t.Fatalf("offer1: %v", err)
	}
	if _, err := repo.UpsertOffer(ctx, Offer{DatasetID: ds2, Enabled: true, AllowFederated: true,
		TrustLevel: TrustL1, MaxOutputBytes: 10, MaxOutputFiles: 4}); err != nil {
		t.Fatalf("offer2: %v", err)
	}

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x/file"}, 2, 16))
	defer svc.Close()

	ent1, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds1, BuyerID: buyer, JobsQuota: 3})
	ent2, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds2, BuyerID: buyer, JobsQuota: 3})

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: []string{ds1, ds2},
	})
	if err != nil {
		t.Fatalf("submit federated: %v", err)
	}

	failed := waitFedStatus(t, repo, fed.ID, FedFailed, 10*time.Second)
	if failed.FailureCode == "" {
		t.Fatalf("failed federated job should carry a failure code: %+v", failed)
	}
	// Both participants refunded to zero usage.
	e1, _ := repo.GetEntitlement(ctx, ent1.ID)
	e2, _ := repo.GetEntitlement(ctx, ent2.ID)
	if e1.JobsUsed != 0 || e2.JobsUsed != 0 {
		t.Fatalf("federated failure did not refund both: e1=%d e2=%d, want 0/0", e1.JobsUsed, e2.JobsUsed)
	}
}

// waitFedStatus polls a federated job until it reaches want or times out.
func waitFedStatus(t *testing.T, repo Repository, fedID, want string, timeout time.Duration) FederatedJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	terminal := map[string]bool{FedReleased: true, FedFailed: true, FedRejected: true}
	for {
		f, err := repo.GetFederatedJob(context.Background(), fedID)
		if err != nil {
			t.Fatalf("get federated job: %v", err)
		}
		if f.Status == want {
			return f
		}
		if terminal[f.Status] && f.Status != want {
			t.Fatalf("federated job reached terminal %q, want %q (code=%q)", f.Status, want, f.FailureCode)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for federated %s to reach %q (now %q)", fedID, want, f.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
