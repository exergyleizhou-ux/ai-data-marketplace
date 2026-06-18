package compute

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// TestComputeRepoIntegration exercises the compute repository against a real
// Postgres (migration 000010): the algorithm registry, offers, atomic quota
// spend (incl. a concurrency race), the job state machine, idempotent submit,
// lease reclaim (crash recovery), the DP ledger, and refund→revoke linkage.
//
// Skips unless DATABASE_URL is set; CI's backend job sets it.
func TestComputeRepoIntegration(t *testing.T) {
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
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	seller := seedUser(t, pool, "cseller", "seller")
	buyer := seedUser(t, pool, "cbuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	// --- algorithm registry ---
	algo, err := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg-" + uniq, Runtime: RuntimeSklearn, Image: "registry/logreg",
		ImageDigest: "sha256:deadbeef", Version: 2, SourceRef: "git:logreg@2",
		OutputKind: OutputModel, Status: AlgoPending, ParamsSchema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("register algorithm: %v", err)
	}
	if algo.Version != 2 || algo.ImageDigest != "sha256:deadbeef" {
		t.Fatalf("algo round-trip wrong: %+v", algo)
	}
	approved, err := repo.ReviewAlgorithm(ctx, algo.ID, AlgoApproved, true)
	if err != nil || approved.Status != AlgoApproved || !approved.Trusted {
		t.Fatalf("review algorithm: %+v err=%v", approved, err)
	}
	list, err := repo.ListApprovedAlgorithms(ctx)
	if err != nil || len(list) == 0 {
		t.Fatalf("list approved: %v (n=%d)", err, len(list))
	}

	// --- offer upsert/get (array + DP epsilon round-trip) ---
	eps, total := 1.5, 6.0
	off, err := repo.UpsertOffer(ctx, Offer{
		DatasetID: dsID, Enabled: true, AllowCustom: false, AllowedAlgoIDs: []string{algo.ID},
		PriceCents: 2000, MaxOutputBytes: 1 << 20, MaxOutputFiles: 4,
		DPEpsilon: &eps, DPEpsilonTotal: &total, TrustLevel: TrustL1,
	})
	if err != nil {
		t.Fatalf("upsert offer: %v", err)
	}
	if len(off.AllowedAlgoIDs) != 1 || off.AllowedAlgoIDs[0] != algo.ID {
		t.Fatalf("offer array round-trip wrong: %+v", off.AllowedAlgoIDs)
	}
	if off.DPEpsilon == nil || *off.DPEpsilon != 1.5 || off.DPEpsilonTotal == nil || *off.DPEpsilonTotal != 6.0 {
		t.Fatalf("offer dp round-trip wrong: %+v", off)
	}
	got, err := repo.GetOffer(ctx, dsID)
	if err != nil || !got.Enabled {
		t.Fatalf("get offer: %+v err=%v", got, err)
	}

	// --- entitlement + atomic quota concurrency ---
	ent, err := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 5})
	if err != nil {
		t.Fatalf("create entitlement: %v", err)
	}
	var ok int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.SpendQuota(ctx, ent.ID); err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 5 {
		t.Fatalf("atomic SpendQuota over/under-spent: %d succeeded, want 5", ok)
	}
	after, _ := repo.GetEntitlement(ctx, ent.ID)
	if after.JobsUsed != 5 || after.Status != EntExhausted {
		t.Fatalf("entitlement after spend: used=%d status=%s", after.JobsUsed, after.Status)
	}
	if _, err := repo.SpendQuota(ctx, ent.ID); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("spend on exhausted: err=%v, want ErrQuotaExhausted", err)
	}

	// --- job state machine: create → claim → release (idempotent) ---
	jent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, JobsQuota: 10})
	job, err := repo.CreateJob(ctx, Job{
		DatasetID: dsID, BuyerID: buyer, EntitlementID: jent.ID,
		AlgorithmID: algo.ID, AlgorithmVersion: algo.Version, Status: JobQueued,
		Params: map[string]any{"target": "y"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.Status != JobQueued || job.AlgorithmVersion != 2 {
		t.Fatalf("job create wrong: %+v", job)
	}
	claimed, err := repo.ClaimJob(ctx, job.ID, "runner-A", 120)
	if err != nil || claimed.Status != JobRunning || claimed.Attempts != 1 {
		t.Fatalf("claim: %+v err=%v", claimed, err)
	}
	// A second claim must fail (already running).
	if _, err := repo.ClaimJob(ctx, job.ID, "runner-B", 120); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("double claim: err=%v, want ErrBadTransition", err)
	}
	if err := repo.Heartbeat(ctx, job.ID, "runner-A", 120); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	rel, err := repo.Release(ctx, job.ID, "outputs/"+job.ID+"/model.pkl", OutputModel, 4096, "")
	if err != nil || rel.Status != JobReleased || rel.OutputBytes != 4096 {
		t.Fatalf("release: %+v err=%v", rel, err)
	}
	// Idempotent re-release returns the released job, no error.
	rel2, err := repo.Release(ctx, job.ID, "outputs/x", OutputModel, 999, "")
	if err != nil || rel2.OutputBytes != 4096 {
		t.Fatalf("idempotent re-release changed output: %+v err=%v", rel2, err)
	}

	// --- idempotent submit (unique key) ---
	k := "idem-" + uniq
	j1, err := repo.CreateJob(ctx, Job{DatasetID: dsID, BuyerID: buyer, EntitlementID: jent.ID, Status: JobQueued}.WithIdempotencyKey(k))
	if err != nil {
		t.Fatalf("idem job1: %v", err)
	}
	if _, err := repo.CreateJob(ctx, Job{DatasetID: dsID, BuyerID: buyer, EntitlementID: jent.ID, Status: JobQueued}.WithIdempotencyKey(k)); !errors.Is(err, ErrDuplicateJob) {
		t.Fatalf("idem job2: err=%v, want ErrDuplicateJob", err)
	}
	back, err := repo.GetJobByIdempotency(ctx, jent.ID, k)
	if err != nil || back.ID != j1.ID {
		t.Fatalf("get by idempotency: %+v err=%v", back, err)
	}

	// --- lease reclaim (crash recovery) ---
	reJob, _ := repo.CreateJob(ctx, Job{DatasetID: dsID, BuyerID: buyer, EntitlementID: jent.ID, Status: JobQueued})
	if _, err := repo.ClaimJob(ctx, reJob.ID, "runner-crash", 120); err != nil {
		t.Fatalf("claim reJob: %v", err)
	}
	// Force the lease into the past (simulate a crashed runner).
	if _, err := pool.Exec(ctx, `UPDATE compute_jobs SET lease_until = now() - interval '1 minute' WHERE id=$1`, reJob.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	n, err := repo.ReclaimStaleLeases(ctx, DefaultMaxAttempts)
	if err != nil || n < 1 {
		t.Fatalf("reclaim: n=%d err=%v", n, err)
	}
	requeued, _ := repo.GetJob(ctx, reJob.ID)
	if requeued.Status != JobQueued {
		t.Fatalf("reclaimed job status=%s, want queued", requeued.Status)
	}
	// Now exhaust attempts and verify reclaim fails it.
	if _, err := repo.ClaimJob(ctx, reJob.ID, "runner-crash", 120); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE compute_jobs SET lease_until = now() - interval '1 minute', attempts=$2 WHERE id=$1`, reJob.ID, DefaultMaxAttempts); err != nil {
		t.Fatalf("exhaust attempts: %v", err)
	}
	if _, err := repo.ReclaimStaleLeases(ctx, DefaultMaxAttempts); err != nil {
		t.Fatalf("reclaim2: %v", err)
	}
	failed, _ := repo.GetJob(ctx, reJob.ID)
	if failed.Status != JobFailed {
		t.Fatalf("exhausted job status=%s, want failed", failed.Status)
	}

	// --- DP ledger ---
	if err := repo.SpendDP(ctx, dsID, buyer, j1.ID, 2.0, nil); err != nil {
		t.Fatalf("spend dp: %v", err)
	}
	if err := repo.SpendDP(ctx, dsID, buyer, "", 1.5, nil); err != nil {
		t.Fatalf("spend dp2: %v", err)
	}
	sum, err := repo.SumDP(ctx, dsID, buyer)
	if err != nil || sum != 3.5 {
		t.Fatalf("sum dp = %v err=%v, want 3.5", sum, err)
	}

	// --- refund → revoke linkage (H2) ---
	orderID := seedOrder(t, pool, buyer, seller, dsID)
	ordEnt, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, OrderID: orderID, JobsQuota: 3})
	// One entitlement per order (migration 000011 unique index) → second grant
	// for the same order is rejected (makes grant-on-payment idempotent).
	if _, err := repo.CreateEntitlement(ctx, Entitlement{DatasetID: dsID, BuyerID: buyer, OrderID: orderID, JobsQuota: 3}); !errors.Is(err, ErrDuplicateEnt) {
		t.Fatalf("duplicate entitlement for order: err=%v, want ErrDuplicateEnt", err)
	}
	full, _ := repo.GetEntitlement(ctx, ordEnt.ID)
	revoked, err := repo.RevokeByOrder(ctx, full.OrderID)
	if err != nil || revoked != 1 {
		t.Fatalf("revoke by order: n=%d err=%v", revoked, err)
	}
	chk, _ := repo.GetEntitlement(ctx, ordEnt.ID)
	if chk.Status != EntRevoked {
		t.Fatalf("entitlement after revoke: %s, want revoked", chk.Status)
	}
}

// --- seed helpers ---

// uniqAccountSuffix returns 16 hex characters from 8 random bytes.
func uniqAccountSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, prefix, role string) string {
	t.Helper()
	account := prefix + "-" + uniqAccountSuffix() + "@example.com"
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x',$2,'verified')
		 ON CONFLICT (account) DO UPDATE SET role = EXCLUDED.role
		 RETURNING id::text`, account, role).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedDataset(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		 VALUES ($1,'C2D bench','structured','commercial','published') RETURNING id::text`, sellerID).Scan(&id); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	return id
}

func seedOrder(t *testing.T, pool *pgxpool.Pool, buyer, seller, datasetID string) string {
	t.Helper()
	// A version is required by orders.version_id (NOT NULL); create one.
	var versionID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO dataset_versions (dataset_id, version_no) VALUES ($1, 1) RETURNING id::text`, datasetID).Scan(&versionID); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO orders (buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status)
		 VALUES ($1,$2,$3,$4,'commercial',2000,200,1800,'created') RETURNING id::text`,
		buyer, seller, datasetID, versionID).Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return id
}

// TestSpendDP_AtomicUnderConcurrency proves the per-(dataset,buyer) advisory lock
// makes the capped SpendDP atomic: N concurrent spends against a budget that only
// fits K must commit EXACTLY K and never overshoot the cap. Without the lock,
// concurrent read-then-insert lets a burst of jobs all pass the budget check and
// overspend (the bug this fixes).
func TestSpendDP_AtomicUnderConcurrency(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB concurrency test")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	repo := NewRepository(pool)
	ctx := context.Background()

	seller := seedUser(t, pool, "dpc-s", "seller")
	buyer := seedUser(t, pool, "dpc-b", "buyer")
	ds := seedDataset(t, pool, seller)
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM dp_budget_ledger WHERE dataset_id=$1 AND buyer_id=$2`, ds, buyer)
		pool.Exec(ctx, `DELETE FROM datasets WHERE id=$1`, ds)
		pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, seller, buyer)
	})

	const eps = 1.0
	const N = 24
	total := 5.0 // budget fits exactly 5 spends of 1.0
	var committed, rejected int64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := repo.SpendDP(ctx, ds, buyer, "", eps, &total); {
			case err == nil:
				atomic.AddInt64(&committed, 1)
			case errors.Is(err, ErrDPBudgetExceeded):
				atomic.AddInt64(&rejected, 1)
			default:
				t.Errorf("unexpected SpendDP error: %v", err)
			}
		}()
	}
	wg.Wait()

	if committed != 5 {
		t.Errorf("committed spends = %d, want 5 — the advisory lock failed to serialize concurrent spends", committed)
	}
	if rejected != N-5 {
		t.Errorf("rejected = %d, want %d", rejected, N-5)
	}
	sum, err := repo.SumDP(ctx, ds, buyer)
	if err != nil {
		t.Fatal(err)
	}
	if sum > total {
		t.Errorf("ledger sum=%.2f OVERSHOT budget=%.2f under concurrency", sum, total)
	}
	if sum != 5.0 {
		t.Errorf("ledger sum=%.2f, want 5.00", sum)
	}
}
