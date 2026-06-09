package compute

import (
	"archive/zip"
	"bytes"
	"context"
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

// TestComputeDockerE2E drives the WHOLE compute pipeline with the REAL docker
// sandbox runner (not the mock): a job is submitted, the worker stages the
// dataset, the dockerRunner runs a digest-pinned algorithm image under the
// hardening flags, and the buyer downloads a REAL model output.
//
// Gated on a real environment — skips unless ALL of these are set:
//
//	DATABASE_URL          a reachable Postgres
//	COMPUTE_E2E_IMAGE     e.g. localhost:5001/vo-logreg
//	COMPUTE_E2E_DIGEST    e.g. sha256:...
//
// and a `docker` daemon is reachable. Build+push the image first, e.g.:
//
//	docker run -d -p 5001:5000 --name registry registry:2
//	docker build -t localhost:5001/vo-logreg:1 algorithms/logreg
//	docker push localhost:5001/vo-logreg:1   # note the printed sha256 digest
func TestComputeDockerE2E(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	image := os.Getenv("COMPUTE_E2E_IMAGE")
	digest := os.Getenv("COMPUTE_E2E_DIGEST")
	if dsn == "" || image == "" || digest == "" {
		t.Skip("set DATABASE_URL + COMPUTE_E2E_IMAGE + COMPUTE_E2E_DIGEST to run the real-docker e2e")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable; skipping real-docker e2e")
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

	seller := seedUser(t, pool, "d2eseller", "seller")
	buyer := seedUser(t, pool, "d2ebuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	// Put a REAL training CSV at the dataset's object key (the worker stages it
	// into the container at /data).
	dataKey := fmt.Sprintf("datasets/%s/data.csv", dsID)
	csv := buildCSV()
	if _, err := uploadOutput(ctx, store, dataKey, csv); err != nil {
		t.Fatalf("seed dataset object: %v", err)
	}

	// A digest-pinned, trusted algorithm pointing at the pushed image.
	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-e2e", Runtime: RuntimeSklearn, Image: image, ImageDigest: digest,
		SourceRef: "git:logreg-e2e", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, TrustLevel: TrustL1, PriceCents: 1000,
		MaxOutputBytes: 1 << 20, MaxRuntimeSecs: 120,
	}); err != nil {
		t.Fatalf("offer: %v", err)
	}

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewDockerRunner(DefaultDockerResources), store, fakeData{key: dataKey}, 1, 4))
	defer svc.Close()

	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 1})
	job, err := svc.SubmitJob(ctx, buyer, SubmitInput{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Real docker pull + run can take a while; give it room.
	released := waitStatus(t, repo, job.ID, JobReleased, 90*time.Second)
	if released.OutputKey == "" || released.OutputBytes == 0 || released.OutputKind != OutputModel {
		t.Fatalf("released job missing output: %+v", released)
	}

	// Download the output and confirm it is the REAL logreg bundle.
	rc, size, _, err := svc.OpenOutput(ctx, buyer, job.ID)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if int64(len(body)) != size {
		t.Fatalf("output size mismatch read=%d size=%d", len(body), size)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("output is not a zip (real logreg writes a zip): %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["model.json"] || !names["metrics.json"] {
		t.Fatalf("output zip missing real model bundle, got: %v", names)
	}
	t.Logf("REAL docker sandbox produced a %d-byte model bundle: %v", size, names)
}

func buildCSV() []byte {
	var b bytes.Buffer
	b.WriteString("f1,f2,label\n")
	// deterministic, linearly separable
	for i := 0; i < 200; i++ {
		x1 := float64(i%20) - 10
		x2 := float64((i*7)%20) - 10
		label := 0
		if x1+x2 > 0 {
			label = 1
		}
		b.WriteString(strconv.FormatFloat(x1, 'f', 4, 64) + "," + strconv.FormatFloat(x2, 'f', 4, 64) + "," + strconv.Itoa(label) + "\n")
	}
	return b.Bytes()
}
