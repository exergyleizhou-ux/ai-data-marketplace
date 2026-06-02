package compute

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// fakeRepo is an in-memory Repository for unit tests. It mirrors the real
// repo's invariants (atomic-ish quota spend, idempotency uniqueness) closely
// enough to exercise the service's business rules without a database.
type fakeRepo struct {
	mu    sync.Mutex
	seq   int
	algos map[string]Algorithm
	offrs map[string]Offer
	ents  map[string]Entitlement
	jobs  map[string]Job
	idem  map[string]string // entitlementID|key -> jobID
	dp    map[string]float64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		algos: map[string]Algorithm{}, offrs: map[string]Offer{}, ents: map[string]Entitlement{},
		jobs: map[string]Job{}, idem: map[string]string{}, dp: map[string]float64{},
	}
}

func (f *fakeRepo) id(prefix string) string { f.seq++; return fmt.Sprintf("%s-%d", prefix, f.seq) }

func (f *fakeRepo) RegisterAlgorithm(_ context.Context, a Algorithm) (Algorithm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.ID == "" {
		a.ID = f.id("algo")
	}
	if a.Version == 0 {
		a.Version = 1
	}
	if a.Status == "" {
		a.Status = AlgoPending
	}
	f.algos[a.ID] = a
	return a, nil
}
func (f *fakeRepo) GetAlgorithm(_ context.Context, id string) (Algorithm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.algos[id]
	if !ok {
		return Algorithm{}, ErrNotFound
	}
	return a, nil
}
func (f *fakeRepo) ListApprovedAlgorithms(_ context.Context) ([]Algorithm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Algorithm
	for _, a := range f.algos {
		if a.Status == AlgoApproved {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListAlgorithmsByStatus(_ context.Context, status string, _ int) ([]Algorithm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Algorithm
	for _, a := range f.algos {
		if a.Status == status {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) ReviewAlgorithm(_ context.Context, id, status string, trusted bool) (Algorithm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.algos[id]
	if !ok {
		return Algorithm{}, ErrNotFound
	}
	a.Status, a.Trusted = status, trusted
	f.algos[id] = a
	return a, nil
}

func (f *fakeRepo) UpsertOffer(_ context.Context, o Offer) (Offer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o.AllowedAlgoIDs == nil {
		o.AllowedAlgoIDs = []string{}
	}
	f.offrs[o.DatasetID] = o
	return o, nil
}
func (f *fakeRepo) GetOffer(_ context.Context, datasetID string) (Offer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.offrs[datasetID]
	if !ok {
		return Offer{}, ErrNotFound
	}
	return o, nil
}

func (f *fakeRepo) CreateEntitlement(_ context.Context, e Entitlement) (Entitlement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Emulate the one-entitlement-per-order unique index (idempotent grant).
	if e.OrderID != "" {
		for _, ex := range f.ents {
			if ex.OrderID == e.OrderID {
				return Entitlement{}, ErrDuplicateEnt
			}
		}
	}
	e.ID = f.id("ent")
	if e.JobsQuota < 1 {
		e.JobsQuota = 1
	}
	e.Status = EntActive
	f.ents[e.ID] = e
	return e, nil
}
func (f *fakeRepo) GetEntitlement(_ context.Context, id string) (Entitlement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.ents[id]
	if !ok {
		return Entitlement{}, ErrNotFound
	}
	return e, nil
}
func (f *fakeRepo) ListEntitlementsByBuyer(_ context.Context, buyerID string, _, _ int) ([]Entitlement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Entitlement
	for _, e := range f.ents {
		if e.BuyerID == buyerID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fakeRepo) SpendQuota(_ context.Context, id string) (Entitlement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.ents[id]
	if !ok {
		return Entitlement{}, ErrNotFound
	}
	// Mirror the real repo's precedence: terminal states first, then quota.
	if e.Status == EntRevoked || e.Status == EntExpired {
		return Entitlement{}, ErrEntitlementState
	}
	if e.JobsUsed >= e.JobsQuota {
		return Entitlement{}, ErrQuotaExhausted
	}
	if e.Status != EntActive {
		return Entitlement{}, ErrEntitlementState
	}
	e.JobsUsed++
	if e.JobsUsed >= e.JobsQuota {
		e.Status = EntExhausted
	}
	f.ents[id] = e
	return e, nil
}
func (f *fakeRepo) RefundQuota(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.ents[id]
	if !ok {
		return ErrNotFound
	}
	if e.JobsUsed > 0 {
		e.JobsUsed--
	}
	if e.Status == EntExhausted {
		e.Status = EntActive
	}
	f.ents[id] = e
	return nil
}
func (f *fakeRepo) RevokeByOrder(_ context.Context, orderID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for id, e := range f.ents {
		if e.OrderID == orderID && (e.Status == EntActive || e.Status == EntExhausted) {
			e.Status = EntRevoked
			f.ents[id] = e
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CreateJob(_ context.Context, j Job) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if j.idemKey != "" {
		k := j.EntitlementID + "|" + j.idemKey
		if _, exists := f.idem[k]; exists {
			return Job{}, ErrDuplicateJob
		}
	}
	j.ID = f.id("job")
	if j.Status == "" {
		j.Status = JobCreated
	}
	f.jobs[j.ID] = j
	if j.idemKey != "" {
		f.idem[j.EntitlementID+"|"+j.idemKey] = j.ID
	}
	return j, nil
}
func (f *fakeRepo) GetJob(_ context.Context, id string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return j, nil
}
func (f *fakeRepo) GetJobByIdempotency(_ context.Context, entitlementID, key string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.idem[entitlementID+"|"+key]
	if !ok {
		return Job{}, ErrNotFound
	}
	return f.jobs[id], nil
}
func (f *fakeRepo) ListJobsByBuyer(_ context.Context, buyerID string, _, _ int) ([]Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Job
	for _, j := range f.jobs {
		if j.BuyerID == buyerID {
			out = append(out, j)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListJobsByStatus(_ context.Context, status string, _ int) ([]Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Job
	for _, j := range f.jobs {
		if j.Status == status {
			out = append(out, j)
		}
	}
	return out, nil
}
func (f *fakeRepo) Transition(_ context.Context, id, from, to string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok || j.Status != from {
		return Job{}, ErrBadTransition
	}
	j.Status = to
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) ClaimJob(_ context.Context, id, runnerID string, _ int) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok || j.Status != JobQueued {
		return Job{}, ErrBadTransition
	}
	j.Status = JobRunning
	j.Attempts++
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) Heartbeat(_ context.Context, id, runnerID string, _ int) error { return nil }
func (f *fakeRepo) Release(_ context.Context, id, key, kind string, b int64, logs string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	if j.Status == JobReleased {
		return j, nil
	}
	j.Status, j.OutputKey, j.OutputKind, j.OutputBytes, j.LogsKey = JobReleased, key, kind, b, logs
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) StageForReview(_ context.Context, id, key, kind string, b int64, logs string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.Status, j.OutputKey, j.OutputKind, j.OutputBytes, j.LogsKey = JobOutputReviewing, key, kind, b, logs
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) Fail(_ context.Context, id, errCode string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.Status, j.Error = JobFailed, errCode
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) Reject(_ context.Context, id, reason string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.Status, j.Error = JobRejected, reason
	f.jobs[id] = j
	return j, nil
}
func (f *fakeRepo) ReclaimStaleLeases(_ context.Context, _ int) (int, error) { return 0, nil }
func (f *fakeRepo) SpendDP(_ context.Context, datasetID, buyerID, _ string, eps float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dp[datasetID+"|"+buyerID] += eps
	return nil
}
func (f *fakeRepo) SumDP(_ context.Context, datasetID, buyerID string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dp[datasetID+"|"+buyerID], nil
}

// --- fake cross-module deps ---

type fakeIdentity struct{ status string }

func (f fakeIdentity) KYCStatus(context.Context, string) (string, error) { return f.status, nil }

type fakeDatasets struct{ info DatasetInfo }

func (f fakeDatasets) ForCompute(context.Context, string) (DatasetInfo, error) { return f.info, nil }

// --- test fixture ---

type fixture struct {
	svc   *Service
	repo  *fakeRepo
	dsID  string
	buyer string
	ent   Entitlement
	algo  Algorithm
}

// newFixture builds a verified buyer, an enabled L1 offer, an approved+trusted
// MODEL algorithm, and a 1-credit entitlement.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(repo, fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: "seller-1", VersionID: "ver-1", Published: true}}, nil)
	ctx := context.Background()

	algo, _ := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "logreg", Runtime: RuntimeSklearn, Image: "logreg", ImageDigest: "sha256:abc",
		SourceRef: "git:logreg@1", OutputKind: OutputModel, Status: AlgoApproved, Trusted: true,
	})
	o, err := repo.UpsertOffer(ctx, Offer{DatasetID: "ds-1", Enabled: true, TrustLevel: TrustL1, PriceCents: 1000})
	if err != nil {
		t.Fatal(err)
	}
	_ = o
	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: "ds-1", BuyerID: "buyer-1", JobsQuota: 1})
	return &fixture{svc: svc, repo: repo, dsID: "ds-1", buyer: "buyer-1", ent: ent, algo: algo}
}

func (fx *fixture) submit() (Job, error) {
	return fx.svc.SubmitJob(context.Background(), fx.buyer, SubmitInput{
		DatasetID: fx.dsID, EntitlementID: fx.ent.ID, AlgorithmID: fx.algo.ID,
	})
}

func TestSubmitJob_Happy(t *testing.T) {
	fx := newFixture(t)
	j, err := fx.submit()
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if j.Status != JobQueued {
		t.Fatalf("status = %q, want queued", j.Status)
	}
	if j.AlgorithmVersion != fx.algo.Version || j.VersionID != "ver-1" {
		t.Fatalf("job not pinned: algoVer=%d versionID=%q", j.AlgorithmVersion, j.VersionID)
	}
	ent, _ := fx.repo.GetEntitlement(context.Background(), fx.ent.ID)
	if ent.JobsUsed != 1 || ent.Status != EntExhausted {
		t.Fatalf("quota not spent: used=%d status=%s", ent.JobsUsed, ent.Status)
	}
}

func TestSubmitJob_L1ModelRequiresTrusted(t *testing.T) {
	fx := newFixture(t)
	// Demote the algorithm to untrusted: L1 model output must be refused.
	if _, err := fx.repo.ReviewAlgorithm(context.Background(), fx.algo.ID, AlgoApproved, false); err != nil {
		t.Fatal(err)
	}
	_, err := fx.submit()
	if !errors.Is(err, ErrModelNeedsTrust) {
		t.Fatalf("err = %v, want ErrModelNeedsTrust", err)
	}
	// And no credit was spent.
	ent, _ := fx.repo.GetEntitlement(context.Background(), fx.ent.ID)
	if ent.JobsUsed != 0 {
		t.Fatalf("quota spent on rejected submit: used=%d", ent.JobsUsed)
	}
}

func TestSubmitJob_UnapprovedAlgorithmRejected(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.repo.ReviewAlgorithm(context.Background(), fx.algo.ID, AlgoPending, false); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.submit(); !errors.Is(err, ErrAlgoNotAllowed) {
		t.Fatalf("err = %v, want ErrAlgoNotAllowed", err)
	}
}

func TestSubmitJob_OfferAllowlistEnforced(t *testing.T) {
	fx := newFixture(t)
	// Offer restricts to some OTHER algorithm id → our algo is not allowed.
	if _, err := fx.repo.UpsertOffer(context.Background(), Offer{
		DatasetID: fx.dsID, Enabled: true, TrustLevel: TrustL1, AllowedAlgoIDs: []string{"some-other-algo"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.submit(); !errors.Is(err, ErrAlgoNotAllowed) {
		t.Fatalf("err = %v, want ErrAlgoNotAllowed", err)
	}
}

func TestSubmitJob_CustomNotAllowed(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.svc.SubmitJob(context.Background(), fx.buyer, SubmitInput{
		DatasetID: fx.dsID, EntitlementID: fx.ent.ID, // no AlgorithmID = custom
	})
	if !errors.Is(err, ErrCustomNotAllowed) {
		t.Fatalf("err = %v, want ErrCustomNotAllowed", err)
	}
}

func TestSubmitJob_OfferDisabled(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.repo.UpsertOffer(context.Background(), Offer{DatasetID: fx.dsID, Enabled: false, TrustLevel: TrustL1}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.submit(); !errors.Is(err, ErrOfferDisabled) {
		t.Fatalf("err = %v, want ErrOfferDisabled", err)
	}
}

func TestSubmitJob_QuotaExhausted(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.submit(); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if _, err := fx.submit(); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("second submit err = %v, want ErrQuotaExhausted", err)
	}
}

func TestSubmitJob_SelfPurchaseRejected(t *testing.T) {
	fx := newFixture(t)
	// Make the buyer the dataset's seller.
	fx.svc = NewService(fx.repo, fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: fx.buyer, VersionID: "ver-1", Published: true}}, nil)
	if _, err := fx.submit(); !errors.Is(err, ErrSelfPurchase) {
		t.Fatalf("err = %v, want ErrSelfPurchase", err)
	}
}

func TestSubmitJob_NotVerified(t *testing.T) {
	fx := newFixture(t)
	fx.svc = NewService(fx.repo, fakeIdentity{status: "pending"},
		fakeDatasets{info: DatasetInfo{SellerID: "seller-1", VersionID: "ver-1", Published: true}}, nil)
	if _, err := fx.submit(); !errors.Is(err, ErrNotVerified) {
		t.Fatalf("err = %v, want ErrNotVerified", err)
	}
}

func TestSubmitJob_WrongBuyerEntitlement(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.svc.SubmitJob(context.Background(), "someone-else", SubmitInput{
		DatasetID: fx.dsID, EntitlementID: fx.ent.ID, AlgorithmID: fx.algo.ID,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestSubmitJob_DPBudgetExceeded(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	svc := NewService(repo, fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: "seller-1", VersionID: "ver-1", Published: true}}, nil)
	algo, _ := repo.RegisterAlgorithm(ctx, Algorithm{
		Name: "stats", Runtime: RuntimeSklearn, Image: "stats", OutputKind: OutputAggregate, Status: AlgoApproved,
	})
	eps, total := 4.0, 5.0
	if _, err := repo.UpsertOffer(ctx, Offer{DatasetID: "ds-1", Enabled: true, TrustLevel: TrustL1,
		DPEpsilon: &eps, DPEpsilonTotal: &total}); err != nil {
		t.Fatal(err)
	}
	ent, _ := repo.CreateEntitlement(ctx, Entitlement{DatasetID: "ds-1", BuyerID: "buyer-1", JobsQuota: 5})
	// Pre-spend 3 of a 5.0 budget; another 4.0 would exceed.
	_ = repo.SpendDP(ctx, "ds-1", "buyer-1", "", 3.0)
	_, err := svc.SubmitJob(ctx, "buyer-1", SubmitInput{DatasetID: "ds-1", EntitlementID: ent.ID, AlgorithmID: algo.ID})
	if !errors.Is(err, ErrDPBudgetExceeded) {
		t.Fatalf("err = %v, want ErrDPBudgetExceeded", err)
	}
}

func TestSubmitJob_Idempotent(t *testing.T) {
	fx := newFixture(t)
	// Give 2 credits so a non-idempotent double-submit WOULD spend twice.
	if _, err := fx.repo.UpsertOffer(context.Background(), Offer{DatasetID: fx.dsID, Enabled: true, TrustLevel: TrustL1}); err != nil {
		t.Fatal(err)
	}
	ent, _ := fx.repo.CreateEntitlement(context.Background(), Entitlement{DatasetID: fx.dsID, BuyerID: fx.buyer, JobsQuota: 2})
	in := SubmitInput{DatasetID: fx.dsID, EntitlementID: ent.ID, AlgorithmID: fx.algo.ID, IdempotencyKey: "k-123"}
	j1, err := fx.svc.SubmitJob(context.Background(), fx.buyer, in)
	if err != nil {
		t.Fatalf("submit1: %v", err)
	}
	j2, err := fx.svc.SubmitJob(context.Background(), fx.buyer, in)
	if err != nil {
		t.Fatalf("submit2: %v", err)
	}
	if j1.ID != j2.ID {
		t.Fatalf("idempotency broken: %s != %s", j1.ID, j2.ID)
	}
	got, _ := fx.repo.GetEntitlement(context.Background(), ent.ID)
	if got.JobsUsed != 1 {
		t.Fatalf("idempotent resubmit spent extra quota: used=%d, want 1", got.JobsUsed)
	}
}

func TestCancelJob_RefundsQuota(t *testing.T) {
	fx := newFixture(t)
	j, err := fx.submit()
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	canceled, err := fx.svc.CancelJob(context.Background(), fx.buyer, j.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled.Status != JobCanceled {
		t.Fatalf("status = %q, want canceled", canceled.Status)
	}
	ent, _ := fx.repo.GetEntitlement(context.Background(), fx.ent.ID)
	if ent.JobsUsed != 0 || ent.Status != EntActive {
		t.Fatalf("quota not refunded: used=%d status=%s", ent.JobsUsed, ent.Status)
	}
}

func TestConfigureOffer_OwnerOnly(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.svc.ConfigureOffer(context.Background(), "not-the-seller", fx.dsID, OfferInput{Enabled: true}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if _, err := fx.svc.ConfigureOffer(context.Background(), "seller-1", fx.dsID, OfferInput{Enabled: true, PriceCents: 500}); err != nil {
		t.Fatalf("owner configure: %v", err)
	}
}

func TestRegisterAlgorithm_TrustedNeedsDigest(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.svc.RegisterAlgorithm(context.Background(), Algorithm{
		Name: "x", Runtime: RuntimeSklearn, Image: "x", OutputKind: OutputModel, Trusted: true, // no digest/source
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
