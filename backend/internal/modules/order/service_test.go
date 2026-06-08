package order

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	orders  map[string]Order
	reviews map[string]Review
	seq     int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{orders: map[string]Order{}} }

func (r *fakeRepo) Create(_ context.Context, o Order) (Order, error) {
	// Emulate the active-order unique constraint.
	for _, e := range r.orders {
		if e.BuyerID == o.BuyerID && e.DatasetID == o.DatasetID &&
			e.Status != StatusRefunded && e.Status != StatusCancelled && e.Status != StatusSettled {
			return Order{}, ErrDuplicateOrder
		}
	}
	r.seq++
	o.ID = "ord-" + itoa(r.seq)
	o.Status = StatusCreated
	if o.ProductType == "" {
		o.ProductType = ProductDownload
	}
	r.orders[o.ID] = o
	return o, nil
}
func (r *fakeRepo) CreateCompute(_ context.Context, o Order) (Order, error) {
	for _, e := range r.orders {
		if e.BuyerID == o.BuyerID && e.DatasetID == o.DatasetID && e.ProductType == ProductCompute &&
			e.Status != StatusRefunded && e.Status != StatusCancelled && e.Status != StatusSettled {
			return Order{}, ErrDuplicateOrder
		}
	}
	r.seq++
	o.ID = "ord-" + itoa(r.seq)
	o.Status = StatusCreated
	o.ProductType = ProductCompute
	r.orders[o.ID] = o
	return o, nil
}
func (r *fakeRepo) GetByID(_ context.Context, id string) (Order, error) {
	o, ok := r.orders[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	return o, nil
}
func (r *fakeRepo) ListByBuyer(_ context.Context, b string, _, _ int) ([]Order, error) {
	return r.filter(func(o Order) bool { return o.BuyerID == b }), nil
}
func (r *fakeRepo) ListBySeller(_ context.Context, s string, _, _ int) ([]Order, error) {
	return r.filter(func(o Order) bool { return o.SellerID == s }), nil
}
func (r *fakeRepo) filter(pred func(Order) bool) []Order {
	var out []Order
	for _, o := range r.orders {
		if pred(o) {
			out = append(out, o)
		}
	}
	return out
}
func (r *fakeRepo) Transition(_ context.Context, id, from, to string, setAutoConfirm bool) (Order, error) {
	o, ok := r.orders[id]
	if !ok || o.Status != from {
		return Order{}, ErrBadTransition
	}
	o.Status = to
	if setAutoConfirm {
		o.AutoConfirmAt = "2026-01-08T00:00:00Z"
	}
	r.orders[id] = o
	return o, nil
}
func (r *fakeRepo) CreateDispute(_ context.Context, _, _, _ string) error     { return nil }
func (r *fakeRepo) ResolveDispute(_ context.Context, _, _, _, _ string) error { return nil }
func (r *fakeRepo) SellerEarnings(_ context.Context, sellerID string) (Earnings, error) {
	var e Earnings
	for _, o := range r.orders {
		if o.SellerID != sellerID {
			continue
		}
		switch o.Status {
		case StatusSettled:
			e.SettledCents += o.SellerAmountCents
			e.SettledOrders++
		case StatusPaid, StatusDelivered, StatusConfirmed:
			e.PendingCents += o.SellerAmountCents
			e.PendingOrders++
		}
	}
	e.WithdrawableCents = e.SettledCents
	return e, nil
}
func (r *fakeRepo) CreateReview(_ context.Context, rv Review) (Review, error) {
	if r.reviews == nil {
		r.reviews = map[string]Review{}
	}
	if _, ok := r.reviews[rv.OrderID]; ok {
		return Review{}, ErrReviewExists
	}
	rv.ID = "rev-" + rv.OrderID
	r.reviews[rv.OrderID] = rv
	return rv, nil
}
func (r *fakeRepo) ListReviewsByDataset(_ context.Context, datasetID string, _, _ int) ([]Review, error) {
	var out []Review
	for _, rv := range r.reviews {
		if rv.DatasetID == datasetID {
			out = append(out, rv)
		}
	}
	return out, nil
}
func (r *fakeRepo) AdminList(_ context.Context, _, _ int) ([]Order, error) {
	var out []Order
	for _, o := range r.orders {
		out = append(out, o)
	}
	return out, nil
}
func (r *fakeRepo) AdminReconciliation(_ context.Context) (Reconciliation, error) {
	var rec Reconciliation
	rec.TotalOrders = int64(len(r.orders))
	for _, o := range r.orders {
		rec.TotalGMV += o.AmountCents
		if o.Status == StatusSettled {
			rec.SettledGMV += o.AmountCents
			rec.PlatformFees += o.PlatformFeeCents
			rec.SettledOrders++
		}
		if o.Status == StatusDisputed {
			rec.DisputedOrders++
		}
		if o.Status == StatusRefunded {
			rec.RefundedOrders++
			rec.RefundedAmount += o.AmountCents
		}
	}
	rec.PendingOrders = rec.TotalOrders - rec.SettledOrders - rec.RefundedOrders - rec.DisputedOrders
	return rec, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type fakeIdentity struct{ verified map[string]bool }

func (f fakeIdentity) KYCStatus(_ context.Context, u string) (string, error) {
	if f.verified[u] {
		return "verified", nil
	}
	return "none", nil
}

type fakeDatasets struct{ p Purchasable }

func (f fakeDatasets) ForPurchase(_ context.Context, _ string) (Purchasable, error) { return f.p, nil }

func newSvc(buyerVerified bool, ds Purchasable) (*Service, *fakeRepo) {
	repo := newFakeRepo()
	id := fakeIdentity{verified: map[string]bool{}}
	if buyerVerified {
		id.verified["buyer"] = true
	}
	return NewService(repo, id, fakeDatasets{p: ds}, nil), repo
}

func published() Purchasable {
	return Purchasable{SellerID: "seller", VersionID: "v1", PriceCents: 100000, Published: true}
}

func TestCreateGuards(t *testing.T) {
	ctx := context.Background()

	// Unverified buyer.
	svc, _ := newSvc(false, published())
	if _, err := svc.Create(ctx, "buyer", "ds1", "commercial"); !errors.Is(err, ErrNotVerified) {
		t.Fatalf("want ErrNotVerified, got %v", err)
	}

	// Not published.
	svc, _ = newSvc(true, Purchasable{SellerID: "seller", VersionID: "v1", PriceCents: 100, Published: false})
	if _, err := svc.Create(ctx, "buyer", "ds1", "commercial"); !errors.Is(err, ErrNotPurchasable) {
		t.Fatalf("want ErrNotPurchasable, got %v", err)
	}

	// Self purchase.
	svc, _ = newSvc(true, Purchasable{SellerID: "buyer", VersionID: "v1", PriceCents: 100, Published: true})
	if _, err := svc.Create(ctx, "buyer", "ds1", "commercial"); !errors.Is(err, ErrSelfPurchase) {
		t.Fatalf("want ErrSelfPurchase, got %v", err)
	}

	// Bad license.
	svc, _ = newSvc(true, published())
	if _, err := svc.Create(ctx, "buyer", "ds1", "rent"); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreateAndFeeSplit(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published()) // price 100000
	o, err := svc.Create(ctx, "buyer", "ds1", "commercial")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if o.AmountCents != 100000 || o.PlatformFeeCents != 10000 || o.SellerAmountCents != 90000 {
		t.Fatalf("fee split wrong: %+v", o)
	}
	// Duplicate active order rejected.
	if _, err := svc.Create(ctx, "buyer", "ds1", "commercial"); !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("want ErrDuplicateOrder, got %v", err)
	}
}

func TestStateMachineHappyPath(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published())
	o, _ := svc.Create(ctx, "buyer", "ds1", "commercial")

	// Confirm before delivery is illegal.
	if _, err := svc.ConfirmDelivery(ctx, "buyer", o.ID); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("want ErrBadTransition, got %v", err)
	}

	if _, err := svc.MarkPaid(ctx, o.ID); err != nil {
		t.Fatalf("paid: %v", err)
	}
	if _, err := svc.MarkDelivered(ctx, o.ID); err != nil {
		t.Fatalf("delivered: %v", err)
	}
	// Only the buyer can confirm.
	if _, err := svc.ConfirmDelivery(ctx, "intruder", o.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if _, err := svc.ConfirmDelivery(ctx, "buyer", o.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	settled, err := svc.MarkSettled(ctx, o.ID)
	if err != nil || settled.Status != StatusSettled {
		t.Fatalf("settle: %v status=%q", err, settled.Status)
	}
}

func TestReviewRequiresSettled(t *testing.T) {
	ctx := context.Background()
	svc, repo := newSvc(true, published())
	o, _ := svc.Create(ctx, "buyer", "ds1", "commercial")

	// Not settled yet -> rejected.
	if _, err := svc.CreateReview(ctx, "buyer", o.ID, 5, "great", false); !errors.Is(err, ErrNotSettled) {
		t.Fatalf("want ErrNotSettled, got %v", err)
	}
	// Drive to settled.
	_, _ = svc.MarkPaid(ctx, o.ID)
	_, _ = svc.MarkDelivered(ctx, o.ID)
	_, _ = svc.ConfirmDelivery(ctx, "buyer", o.ID)
	_, _ = svc.MarkSettled(ctx, o.ID)

	if _, err := svc.CreateReview(ctx, "buyer", o.ID, 6, "", false); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation for out-of-range score, got %v", err)
	}
	rv, err := svc.CreateReview(ctx, "buyer", o.ID, 5, "干净好用", false)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rv.DatasetID != "ds1" {
		t.Fatalf("review dataset = %q, want ds1", rv.DatasetID)
	}
	// One review per order.
	if _, err := svc.CreateReview(ctx, "buyer", o.ID, 4, "", false); !errors.Is(err, ErrReviewExists) {
		t.Fatalf("want ErrReviewExists, got %v", err)
	}
	_ = repo
}

func TestEarnings(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published()) // price 100000 -> seller 90000
	o, _ := svc.Create(ctx, "buyer", "ds1", "commercial")
	_, _ = svc.MarkPaid(ctx, o.ID) // pending

	e, _ := svc.Earnings(ctx, "seller")
	if e.PendingCents != 90000 || e.SettledCents != 0 {
		t.Fatalf("pending earnings wrong: %+v", e)
	}
	_, _ = svc.MarkDelivered(ctx, o.ID)
	_, _ = svc.ConfirmDelivery(ctx, "buyer", o.ID)
	_, _ = svc.MarkSettled(ctx, o.ID)
	e, _ = svc.Earnings(ctx, "seller")
	if e.SettledCents != 90000 || e.WithdrawableCents != 90000 || e.PendingCents != 0 {
		t.Fatalf("settled earnings wrong: %+v", e)
	}
}

func TestResolveDispute(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published())
	o, _ := svc.Create(ctx, "buyer", "ds1", "commercial")
	_, _ = svc.MarkPaid(ctx, o.ID)
	_, _ = svc.Dispute(ctx, "buyer", o.ID, "质量问题")

	// Resolve as refund.
	got, err := svc.ResolveDispute(ctx, "ops", o.ID, true, "确认退款")
	if err != nil || got.Status != StatusRefunded {
		t.Fatalf("resolve refund: %v status=%q", err, got.Status)
	}
	// Cannot resolve a non-disputed order.
	if _, err := svc.ResolveDispute(ctx, "ops", o.ID, true, ""); !errors.Is(err, ErrNotDisputed) {
		t.Fatalf("want ErrNotDisputed, got %v", err)
	}
}

func TestDispute(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published())
	o, _ := svc.Create(ctx, "buyer", "ds1", "commercial")
	_, _ = svc.MarkPaid(ctx, o.ID)

	d, err := svc.Dispute(ctx, "buyer", o.ID, "数据有问题")
	if err != nil || d.Status != StatusDisputed {
		t.Fatalf("dispute: %v status=%q", err, d.Status)
	}
}

// --- compute (C2D) orders ---

type fakeGranter struct {
	calls   int
	orderID string
	dataset string
	buyer   string
}

func (f *fakeGranter) GrantForOrder(_ context.Context, orderID, datasetID, buyerID string) error {
	f.calls++
	f.orderID, f.dataset, f.buyer = orderID, datasetID, buyerID
	return nil
}

func TestCreateComputeAndPayGrantsAndDelivers(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published())
	g := &fakeGranter{}
	svc.SetComputeGranter(g)

	o, err := svc.CreateCompute(ctx, "buyer", "seller", "ds1", 5000)
	if err != nil {
		t.Fatalf("create compute: %v", err)
	}
	if o.ProductType != ProductCompute || o.VersionID != "" || o.AmountCents != 5000 {
		t.Fatalf("compute order wrong: %+v", o)
	}
	if o.PlatformFeeCents != 500 || o.SellerAmountCents != 4500 {
		t.Fatalf("fee split wrong: fee=%d seller=%d", o.PlatformFeeCents, o.SellerAmountCents)
	}

	paid, err := svc.MarkPaid(ctx, o.ID)
	if err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	// Paying a compute order grants the entitlement and auto-delivers.
	if paid.Status != StatusDelivered {
		t.Fatalf("status = %q, want delivered", paid.Status)
	}
	if g.calls != 1 || g.orderID != o.ID || g.dataset != "ds1" || g.buyer != "buyer" {
		t.Fatalf("granter not called correctly: %+v", g)
	}
}

func TestCreateCompute_SelfPurchaseRejected(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(true, published())
	if _, err := svc.CreateCompute(ctx, "buyer", "buyer", "ds1", 5000); err != ErrSelfPurchase {
		t.Fatalf("err = %v, want ErrSelfPurchase", err)
	}
}

func TestCreateCompute_RequiresVerified(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(false, published()) // buyer not verified
	if _, err := svc.CreateCompute(ctx, "buyer", "seller", "ds1", 5000); err != ErrNotVerified {
		t.Fatalf("err = %v, want ErrNotVerified", err)
	}
}
