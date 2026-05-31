package order

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	orders map[string]Order
	seq    int
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
func (r *fakeRepo) CreateDispute(_ context.Context, _, _, _ string) error { return nil }

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
