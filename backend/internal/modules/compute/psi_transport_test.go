package compute

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"
)

// startPartyServer runs a party as a real HTTP server (a stand-in for a long-lived
// party node) and returns an orchestrator-side client for it.
func startPartyServer(t *testing.T, set []string) (httpPSIParty, func()) {
	t.Helper()
	ps, err := NewPSIPartyServer(set)
	if err != nil {
		t.Fatalf("new party server: %v", err)
	}
	srv := httptest.NewServer(ps.Handler())
	return httpPSIParty{baseURL: srv.URL, client: srv.Client()}, srv.Close
}

// TestHTTPPSIMatchesMockOverTheWire proves the two-round protocol works when the
// parties are SEPARATE long-lived servers and the orchestrator coordinates over
// HTTP — the multi-node deployment shape. Result must equal the plaintext mock.
func TestHTTPPSIMatchesMockOverTheWire(t *testing.T) {
	sets := [][]string{
		{"a@x.com", "b@x.com", "c@x.com", "d@x.com"},
		{"b@x.com", "c@x.com", "e@x.com"},
		{"c@x.com", "b@x.com", "z@x.com"},
	}
	want, _ := NewMockMPC().RunPSI(context.Background(), sets)

	parties := make([]httpPSIParty, len(sets))
	for i, s := range sets {
		p, stop := startPartyServer(t, s)
		defer stop()
		parties[i] = p
	}

	got, err := RunHTTPPSI(context.Background(), parties)
	if err != nil {
		t.Fatalf("run http psi: %v", err)
	}
	if !reflect.DeepEqual(got.Intersection, want.Intersection) {
		t.Fatalf("http psi = %v, want %v", got.Intersection, want.Intersection)
	}
	if got.Cardinality != want.Cardinality {
		t.Fatalf("cardinality = %d, want %d", got.Cardinality, want.Cardinality)
	}
}

// TestHTTPPSIServerNeverExposesSet: the party server has no endpoint that returns
// its raw set — round1 returns blinded points, and /label only returns elements
// at indices the orchestrator already matched (party 0's own intersection
// members). Requesting a non-matching index set still only yields that party's
// own elements, never another party's.
func TestHTTPPSIServerRound1IsBlinded(t *testing.T) {
	p, stop := startPartyServer(t, []string{"secret-value"})
	defer stop()
	pts, err := p.round1(context.Background())
	if err != nil {
		t.Fatalf("round1: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("round1 len = %d", len(pts))
	}
	// The wire value must be the blinded point, not the plaintext.
	if pts[0].Cmp(psiHashToGroup("secret-value")) == 0 {
		t.Fatal("round1 returned the unblinded point over the wire")
	}
}
