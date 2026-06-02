package compute

import "testing"

func f64(v float64) *float64 { return &v }

func TestEffectiveParams_PlatformOwnsEpsilon(t *testing.T) {
	// A buyer-supplied _epsilon is dropped; the offer's epsilon is injected.
	j := Job{Params: map[string]any{"columns": []any{"age"}, "_epsilon": 999.0}, DPEpsilon: f64(2.0)}
	p := effectiveParams(j)
	if p["_epsilon"] != 2.0 {
		t.Fatalf("_epsilon = %v, want platform 2.0 (buyer 999 must be dropped)", p["_epsilon"])
	}
	if _, ok := p["columns"]; !ok {
		t.Fatal("buyer's non-epsilon params must be preserved")
	}
}

func TestEffectiveParams_NoBudgetNoEpsilon(t *testing.T) {
	j := Job{Params: map[string]any{"_epsilon": 999.0}} // no DPEpsilon
	p := effectiveParams(j)
	if _, ok := p["_epsilon"]; ok {
		t.Fatalf("no offer epsilon → _epsilon must be absent, got %v", p["_epsilon"])
	}
}
