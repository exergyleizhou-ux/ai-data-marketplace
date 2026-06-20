package compute

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// TestOfferSignals: the public catalog signal that surfaces compute-to-data
// discoverability. For a batch of dataset ids it returns, only for datasets with
// an ENABLED offer, the trust level + federated/psi flags + the count of released
// compute jobs (a usage/confidence signal). Datasets without an enabled offer are
// absent from the map (the catalog shows no C2D badge for them).
func TestOfferSignals(t *testing.T) {
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

	seller := seedUser(t, pool, "sigseller", "seller")
	buyer := seedUser(t, pool, "sigbuyer", "buyer")
	dsA := seedDataset(t, pool, seller) // has an enabled L2 + federated offer + 2 released jobs
	dsB := seedDataset(t, pool, seller) // no offer at all

	if _, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsA, Enabled: true, TrustLevel: TrustL2, AllowFederated: true,
		PriceCents: 1000, MaxOutputBytes: 1 << 20,
	}); err != nil {
		t.Fatalf("offer A: %v", err)
	}
	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsA, BuyerID: buyer, JobsQuota: 5})
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO compute_jobs (id, dataset_id, buyer_id, entitlement_id, status, attempts)
			 VALUES (gen_random_uuid(), $1, $2, $3, 'released', 0)`, dsA, buyer, ent.ID); err != nil {
			t.Fatalf("insert released job: %v", err)
		}
	}

	sigs, err := repo.OfferSignals(ctx, []string{dsA, dsB})
	if err != nil {
		t.Fatalf("OfferSignals: %v", err)
	}
	a, ok := sigs[dsA]
	if !ok {
		t.Fatalf("dataset A with an enabled offer must have a signal")
	}
	if !a.Enabled || a.TrustLevel != TrustL2 || !a.AllowFederated {
		t.Errorf("A signal = %+v, want enabled L2 + federated", a)
	}
	if a.JobsRun != 2 {
		t.Errorf("A jobs_run = %d, want 2", a.JobsRun)
	}
	if _, ok := sigs[dsB]; ok {
		t.Errorf("dataset B with no offer must NOT have a signal, got %+v", sigs[dsB])
	}
}
