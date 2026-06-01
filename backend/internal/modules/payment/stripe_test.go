package payment

import (
	"context"
	"errors"
	"testing"
)

// fakePayoutStore is an in-memory PayoutAccountStore for offline tests.
type fakePayoutStore struct {
	refs   map[string]string // sellerID -> acct ref
	getErr error
	saved  []string // sellerIDs persisted, in order
}

func (f *fakePayoutStore) PayoutAccountRef(_ context.Context, sellerID, _ string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.refs[sellerID], nil
}

func (f *fakePayoutStore) SavePayoutAccount(_ context.Context, sellerID, _, accountRef string) error {
	if f.refs == nil {
		f.refs = map[string]string{}
	}
	f.refs[sellerID] = accountRef
	f.saved = append(f.saved, sellerID)
	return nil
}

// TestEnsureAccountUsesPersistedRef verifies the persisted mapping (H1) is used
// without ever calling the Stripe API: a non-empty stored ref short-circuits
// account creation, so this runs fully offline.
func TestEnsureAccountUsesPersistedRef(t *testing.T) {
	store := &fakePayoutStore{refs: map[string]string{"seller-1": "acct_persisted"}}
	p := NewStripeProvider("sk_test_dummy", "", "usd", store)

	got, err := p.ensureAccount(context.Background(), "seller-1")
	if err != nil {
		t.Fatalf("ensureAccount: %v", err)
	}
	if got != "acct_persisted" {
		t.Fatalf("got %q, want acct_persisted", got)
	}
	if len(store.saved) != 0 {
		t.Fatalf("expected no new account persisted, got %v", store.saved)
	}

	// Cached on the provider after first resolution.
	if id, ok := p.accounts["seller-1"]; !ok || id != "acct_persisted" {
		t.Fatalf("expected in-process cache to hold acct_persisted, got %q (ok=%v)", id, ok)
	}
}

// TestEnsureAccountStoreErrorPropagates ensures a store read failure surfaces
// instead of silently falling through to (network) account creation.
func TestEnsureAccountStoreErrorPropagates(t *testing.T) {
	store := &fakePayoutStore{getErr: errors.New("db down")}
	p := NewStripeProvider("sk_test_dummy", "", "usd", store)

	if _, err := p.ensureAccount(context.Background(), "seller-x"); err == nil {
		t.Fatal("expected error from store read, got nil")
	}
}
