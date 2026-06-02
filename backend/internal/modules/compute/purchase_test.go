package compute

import (
	"context"
	"errors"
	"testing"
)

type fakeOrderCreator struct {
	id        string
	err       error
	gotAmount int64
	gotSeller string
}

func (f *fakeOrderCreator) CreateComputeOrder(_ context.Context, buyer, seller, ds string, amount int64) (string, error) {
	f.gotAmount = amount
	f.gotSeller = seller
	return f.id, f.err
}

func TestPurchaseViaOrder_Happy(t *testing.T) {
	fx := newFixture(t)
	oc := &fakeOrderCreator{id: "ord-123"}
	fx.svc.SetOrderCreator(oc)
	id, err := fx.svc.PurchaseViaOrder(context.Background(), fx.buyer, fx.dsID)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if id != "ord-123" {
		t.Fatalf("order id = %q", id)
	}
	if oc.gotAmount != 1000 || oc.gotSeller != "seller-1" {
		t.Fatalf("order priced/seller wrong: amount=%d seller=%q", oc.gotAmount, oc.gotSeller)
	}
}

func TestPurchaseViaOrder_OfferDisabled(t *testing.T) {
	fx := newFixture(t)
	fx.svc.SetOrderCreator(&fakeOrderCreator{id: "x"})
	if _, err := fx.repo.UpsertOffer(context.Background(), Offer{DatasetID: fx.dsID, Enabled: false, TrustLevel: TrustL1}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.PurchaseViaOrder(context.Background(), fx.buyer, fx.dsID); !errors.Is(err, ErrOfferDisabled) {
		t.Fatalf("err = %v, want ErrOfferDisabled", err)
	}
}

func TestPurchaseViaOrder_SelfPurchase(t *testing.T) {
	fx := newFixture(t)
	// Buyer is the dataset seller.
	fx.svc = NewService(fx.repo, fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: fx.buyer, VersionID: "v", Published: true}}, nil)
	fx.svc.SetOrderCreator(&fakeOrderCreator{id: "x"})
	if _, err := fx.svc.PurchaseViaOrder(context.Background(), fx.buyer, fx.dsID); !errors.Is(err, ErrSelfPurchase) {
		t.Fatalf("err = %v, want ErrSelfPurchase", err)
	}
}

func TestPurchaseViaOrder_NotWired(t *testing.T) {
	fx := newFixture(t) // no SetOrderCreator
	if _, err := fx.svc.PurchaseViaOrder(context.Background(), fx.buyer, fx.dsID); !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestGrantForOrder_Idempotent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	if err := fx.svc.GrantForOrder(ctx, "ord-1", fx.dsID, fx.buyer); err != nil {
		t.Fatalf("grant1: %v", err)
	}
	// Second grant for the same order is a no-op (idempotent), not an error.
	if err := fx.svc.GrantForOrder(ctx, "ord-1", fx.dsID, fx.buyer); err != nil {
		t.Fatalf("grant2 (idempotent): %v", err)
	}
	// Exactly one entitlement exists for the buyer.
	ents, _ := fx.repo.ListEntitlementsByBuyer(ctx, fx.buyer, 100, 0)
	n := 0
	for _, e := range ents {
		if e.OrderID == "ord-1" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("idempotent grant created %d entitlements, want 1", n)
	}
}
