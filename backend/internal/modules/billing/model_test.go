package billing

import "testing"

func TestParsePriceMap(t *testing.T) {
	m := parsePriceMap(" price_a:pro , price_b:scale ,bad, price_c: ")
	if m["price_a"] != "pro" || m["price_b"] != "scale" {
		t.Fatalf("map = %+v", m)
	}
	if _, ok := m["bad"]; ok {
		t.Error("entries without ':' must be skipped")
	}
	if _, ok := m["price_c"]; ok {
		t.Error("entries with an empty tier must be skipped")
	}
	if len(parsePriceMap("")) != 0 {
		t.Error("empty string → empty map")
	}
}

func TestResolveTierChange(t *testing.T) {
	pm := map[string]string{"price_pro": "pro", "price_scale": "scale"}
	cases := []struct {
		event, price string
		wantTier     string
		wantApply    bool
	}{
		{"customer.subscription.created", "price_pro", "pro", true},
		{"customer.subscription.updated", "price_scale", "scale", true},
		{"customer.subscription.deleted", "price_pro", "free", true},  // cancel → downgrade
		{"customer.subscription.updated", "price_unknown", "", false}, // unmapped price → ignore
		{"invoice.paid", "price_pro", "", false},                      // irrelevant event
	}
	for _, c := range cases {
		tier, apply := resolveTierChange(c.event, c.price, pm)
		if tier != c.wantTier || apply != c.wantApply {
			t.Errorf("%s/%s → (%q,%v), want (%q,%v)", c.event, c.price, tier, apply, c.wantTier, c.wantApply)
		}
	}
}
