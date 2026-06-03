package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// mapData resolves a per-dataset object key (federated stages a different CSV
// into each party's sandbox).
type mapData struct{ keys map[string]string }

func (m mapData) CurrentObjectKey(_ context.Context, datasetID string) (string, error) {
	return m.keys[datasetID], nil
}

// TestComputeFederatedDockerE2E drives the WHOLE federated pipeline with the REAL
// docker sandbox runner: two datasets, each sub-job trains the digest-pinned
// fed-logreg image in its own `--network none` sandbox emitting fedparams-v1
// local params; the platform aggregates them with real FedAvg and the buyer
// downloads the joint fedmodel-v1. Raw data never leaves a sandbox.
//
// Gated — skips unless ALL set + a docker daemon is reachable:
//
//	DATABASE_URL              a reachable Postgres
//	COMPUTE_FED_E2E_IMAGE     e.g. localhost:5001/vo-fed-logreg
//	COMPUTE_FED_E2E_DIGEST    e.g. sha256:...
//
// Build+push first (see algorithms/fed-logreg/README.md), e.g.:
//
//	docker run -d -p 5001:5000 --name registry registry:2
//	docker build -t localhost:5001/vo-fed-logreg:1 algorithms/fed-logreg
//	docker push localhost:5001/vo-fed-logreg:1   # note the printed sha256 digest
func TestComputeFederatedDockerE2E(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	image := os.Getenv("COMPUTE_FED_E2E_IMAGE")
	digest := os.Getenv("COMPUTE_FED_E2E_DIGEST")
	if dsn == "" || image == "" || digest == "" {
		t.Skip("set DATABASE_URL + COMPUTE_FED_E2E_IMAGE + COMPUTE_FED_E2E_DIGEST to run the federated docker e2e")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable; skipping federated docker e2e")
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
	seller := seedUser(t, pool, fmt.Sprintf("fd2eseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("fd2ebuyer-%d", uniq), "buyer")

	// Two datasets that SHARE the feature schema (FedAvg precondition), with
	// different data, staged at their own object keys.
	keys := map[string]string{}
	var dsIDs, entIDs []string
	for i := 0; i < 2; i++ {
		ds := seedDataset(t, pool, seller)
		key := fmt.Sprintf("datasets/%s/data.csv", ds)
		if _, err := uploadOutput(ctx, store, key, fedCSV(i)); err != nil {
			t.Fatalf("seed dataset %d: %v", i, err)
		}
		keys[ds] = key
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: ds, Enabled: true, AllowFederated: true, TrustLevel: TrustL1,
			PriceCents: 1000, MaxOutputBytes: 1 << 20, MaxRuntimeSecs: 120,
		}); err != nil {
			t.Fatalf("offer %d: %v", i, err)
		}
		ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds, BuyerID: buyer, JobsQuota: 1})
		dsIDs = append(dsIDs, ds)
		entIDs = append(entIDs, ent.ID)
	}

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "fed-logreg-e2e", Runtime: RuntimeFedLogreg, Image: image, ImageDigest: digest,
		SourceRef: "git:fed-logreg-e2e", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, Published: true}},
		nil,
		WithWorker(NewDockerRunner(DefaultDockerResources), store, mapData{keys: keys}, 2, 8))
	defer svc.Close()

	fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: dsIDs,
	})
	if err != nil {
		t.Fatalf("submit federated: %v", err)
	}

	// Two real docker pulls + trainings; give it room.
	released := waitFedStatus(t, repo, fed.ID, FedReleased, 150*time.Second)

	rc, size, _, err := svc.OpenFederatedOutput(ctx, buyer, released.ID)
	if err != nil {
		t.Fatalf("open federated output: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if int64(len(body)) != size {
		t.Fatalf("size mismatch read=%d size=%d", len(body), size)
	}
	var joint struct {
		Format       string    `json:"_format"`
		Weights      []float64 `json:"weights"`
		Intercept    float64   `json:"intercept"`
		NTotal       int       `json:"n_total"`
		Participants int       `json:"participants"`
	}
	if err := json.Unmarshal(body, &joint); err != nil {
		t.Fatalf("joint not JSON: %v", err)
	}
	if joint.Format != "fedmodel-v1" || joint.Participants != 2 || len(joint.Weights) != 2 || joint.NTotal <= 0 {
		t.Fatalf("unexpected joint model: %+v", joint)
	}
	t.Logf("REAL federated docker: 2 sandboxes → FedAvg joint model weights=%v intercept=%.4f n_total=%d",
		joint.Weights, joint.Intercept, joint.NTotal)

	// Sub-job partials must NOT be buyer-downloadable.
	subs, _ := repo.ListSubJobs(ctx, fed.ID)
	if len(subs) != 2 {
		t.Fatalf("want 2 sub-jobs, got %d", len(subs))
	}
	if _, _, _, err := svc.OpenOutput(ctx, buyer, subs[0].ID); err == nil {
		t.Fatal("buyer must not download a federated sub-job partial")
	}
	for i, eid := range entIDs {
		e, _ := repo.GetEntitlement(ctx, eid)
		if e.JobsUsed != 1 {
			t.Fatalf("entitlement %d jobs_used=%d, want 1 (billed)", i, e.JobsUsed)
		}
	}
}

// fedCSV builds a small linearly-separable dataset (shared schema f1,f2,label),
// shifted per party so the two local models differ but remain averageable.
func fedCSV(party int) []byte {
	var b bytes.Buffer
	b.WriteString("f1,f2,label\n")
	for i := 0; i < 80; i++ {
		x1 := float64(i%10) - 5 + float64(party)
		x2 := float64((i*3)%10) - 5
		label := 0
		if x1+x2 > float64(party) {
			label = 1
		}
		b.WriteString(strconv.FormatFloat(x1, 'f', 3, 64) + "," +
			strconv.FormatFloat(x2, 'f', 3, 64) + "," + strconv.Itoa(label) + "\n")
	}
	return b.Bytes()
}
