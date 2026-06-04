package compute

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestPSIRequiresAllowPSI asserts a PSI job needs the dedicated allow_psi seller
// consent — enabling allow_federated (co-train a model) does NOT implicitly allow
// PSI (reveal set overlap), which is a distinct privacy exposure. Also checks the
// allow_psi flag round-trips through the offer repo.
func TestPSIRequiresAllowPSI(t *testing.T) {
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
	seller := seedUser(t, pool, fmt.Sprintf("apseller-%d", uniq), "seller")
	buyer := seedUser(t, pool, fmt.Sprintf("apbuyer-%d", uniq), "buyer")
	ds1 := seedDataset(t, pool, seller)
	ds2 := seedDataset(t, pool, seller)

	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "psi-extract", Runtime: RuntimePSIExtract, Image: "registry/psi", ImageDigest: "sha256:psi",
		SourceRef: "git:psi", OutputKind: OutputAggregate, Status: AlgoApproved, Trusted: true,
	})
	if err != nil {
		t.Fatalf("register algo: %v", err)
	}
	// Offers allow federated but NOT psi.
	for _, ds := range []string{ds1, ds2} {
		if _, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: ds, Enabled: true, AllowFederated: true, AllowPSI: false, TrustLevel: TrustL1,
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

	repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds1, BuyerID: buyer, JobsQuota: 3})
	repo.CreateEntitlement(ctx, Entitlement{DatasetID: ds2, BuyerID: buyer, JobsQuota: 3})

	// PSI submit must be refused: federated is allowed but PSI is not.
	if _, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: []string{ds1, ds2},
	}); !errors.Is(err, ErrOfferDisabled) {
		t.Fatalf("PSI without allow_psi must be refused with ErrOfferDisabled, got %v", err)
	}

	// Enable allow_psi → the flag round-trips and the submit succeeds.
	for _, ds := range []string{ds1, ds2} {
		o, err := repo.UpsertOffer(ctx, Offer{
			DatasetID: ds, Enabled: true, AllowFederated: true, AllowPSI: true, TrustLevel: TrustL1,
			MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		})
		if err != nil {
			t.Fatalf("offer %s: %v", ds, err)
		}
		if !o.AllowPSI {
			t.Fatalf("allow_psi did not round-trip through the offer repo")
		}
	}
	if _, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
		AlgorithmID: algo.ID, DatasetIDs: []string{ds1, ds2},
	}); err != nil {
		t.Fatalf("PSI with allow_psi should succeed: %v", err)
	}
}
