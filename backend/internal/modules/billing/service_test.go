package billing

import (
	"context"
	"testing"

	"github.com/stripe/stripe-go/v79"
)

type fakeTier struct {
	acct, tier string
	n          int
}

func (f *fakeTier) SetTier(_ context.Context, a, t string) (int, error) {
	f.acct, f.tier, f.n = a, t, f.n+1
	return 1, nil
}

func subEvent(eventType, accountID, priceID string) stripe.Event {
	raw := []byte(`{"metadata":{"account_id":"` + accountID + `"},"items":{"data":[{"price":{"id":"` + priceID + `"}}]}}`)
	return stripe.Event{Type: stripe.EventType(eventType), Data: &stripe.EventData{Raw: raw}}
}

func TestApplyVerifiedEvent(t *testing.T) {
	ft := &fakeTier{}
	s := NewService("sk_test_x", "whsec_x", "price_pro:pro,price_scale:scale", "http://x", ft)

	if err := s.applyVerifiedEvent(subEvent("customer.subscription.updated", "acct-1", "price_scale")); err != nil {
		t.Fatal(err)
	}
	if ft.acct != "acct-1" || ft.tier != "scale" {
		t.Fatalf("upgrade → %s/%s, want acct-1/scale", ft.acct, ft.tier)
	}

	// Cancellation downgrades to free.
	if err := s.applyVerifiedEvent(subEvent("customer.subscription.deleted", "acct-1", "price_scale")); err != nil {
		t.Fatal(err)
	}
	if ft.tier != "free" {
		t.Fatalf("cancel → %s, want free", ft.tier)
	}

	// An irrelevant event changes nothing.
	before := ft.n
	_ = s.applyVerifiedEvent(subEvent("invoice.paid", "acct-1", "price_scale"))
	// An unmapped price changes nothing.
	_ = s.applyVerifiedEvent(subEvent("customer.subscription.updated", "acct-1", "price_unknown"))
	if ft.n != before {
		t.Errorf("irrelevant/unmapped events must not change tier (n went %d→%d)", before, ft.n)
	}
}

func TestEnabledAndDisabledPaths(t *testing.T) {
	off := NewService("", "", "", "", &fakeTier{})
	if off.Enabled() {
		t.Error("no keys → disabled")
	}
	if err := off.HandleWebhook([]byte("{}"), ""); err != ErrDisabled {
		t.Errorf("disabled webhook err = %v, want ErrDisabled", err)
	}
	if _, err := off.CheckoutURL(context.Background(), "a", "price_pro"); err != ErrDisabled {
		t.Errorf("disabled checkout err = %v, want ErrDisabled", err)
	}
	on := NewService("sk_test", "whsec", "price_pro:pro", "http://x", &fakeTier{})
	if !on.Enabled() {
		t.Error("both keys → enabled")
	}
	if _, err := on.CheckoutURL(context.Background(), "a", "price_unknown"); err != ErrUnknownPrice {
		t.Errorf("unknown price err = %v, want ErrUnknownPrice", err)
	}
}
