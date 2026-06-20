// Package billing turns Stripe subscription events into Oasis Verify plan tiers.
// It is config-gated: with no Stripe keys it is disabled (the free tier still
// works), so the product ships sellable-ready — going live with paid plans is
// just setting STRIPE_SECRET_KEY / STRIPE_WEBHOOK_SECRET / VERIFY_PRICE_MAP.
package billing

import "strings"

// parsePriceMap parses a "priceID:tier,priceID:tier" string (env VERIFY_PRICE_MAP)
// into a map. Malformed or empty-tier entries are skipped.
func parsePriceMap(s string) map[string]string {
	m := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(pair, ":")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if ok && k != "" && v != "" {
			m[k] = v
		}
	}
	return m
}

// resolveTierChange decides the new plan tier for a Stripe subscription event.
// Returns (tier, true) when the caller should apply it, ("", false) to ignore.
// A canceled subscription downgrades to free; an active subscription maps its
// price to a tier (unmapped prices are ignored — we don't guess).
func resolveTierChange(eventType, priceID string, priceMap map[string]string) (string, bool) {
	switch eventType {
	case "customer.subscription.deleted":
		return "free", true
	case "customer.subscription.created", "customer.subscription.updated":
		if tier, ok := priceMap[priceID]; ok {
			return tier, true
		}
		return "", false
	default:
		return "", false
	}
}
