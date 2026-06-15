package compute

import (
	"context"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

// TestTwoRoundPSIMatchesMock: the two-round protocol — where each party keeps its
// set AND secret private and the orchestrator only ever routes blinded points —
// must yield the same intersection as the plaintext mock, across many trials.
func TestTwoRoundPSIMatchesMock(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	mock := NewMockMPC()
	for trial := 0; trial < 20; trial++ {
		nParties := 2 + rng.Intn(3)
		parties := make([][]string, nParties)
		for i := range parties {
			n := rng.Intn(7)
			set := make([]string, n)
			for j := range set {
				set[j] = "id" + strconv.Itoa(rng.Intn(10))
			}
			parties[i] = set
		}
		want, _ := mock.RunPSI(context.Background(), parties)

		agents := make([]*psiPartyAgent, nParties)
		for i := range agents {
			a, err := newPSIPartyAgent(parties[i])
			if err != nil {
				t.Fatalf("agent %d: %v", i, err)
			}
			agents[i] = a
		}
		got, err := runTwoRoundPSI(agents)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		if !reflect.DeepEqual(got.Intersection, want.Intersection) {
			t.Fatalf("trial %d: two-round=%v want=%v (parties=%v)", trial, got.Intersection, want.Intersection, parties)
		}
	}
}

// TestTwoRoundPSIAgentHidesSetAndSecret: a party's round-1 output is blinded
// (never its plaintext elements), and re-blinding peers' points composes — the
// orchestrator, holding only these points, cannot read any party's set.
func TestTwoRoundPSIAgentHidesSetAndSecret(t *testing.T) {
	a, err := newPSIPartyAgent([]string{"alice", "bob"})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	r1 := a.round1()
	if len(r1) != 2 {
		t.Fatalf("round1 len = %d", len(r1))
	}
	// Each round-1 point must differ from the bare hash-to-group of the element
	// (i.e. it is actually blinded with the secret).
	for i, e := range []string{"alice", "bob"} {
		if r1[i].Cmp(psiHashToGroup(e)) == 0 {
			t.Fatalf("round1[%d] is the unblinded point — secret not applied", i)
		}
	}
	// Re-blinding by another agent's secret changes the points further (the
	// two-round composition), proving each party contributes its own exponent.
	b, _ := newPSIPartyAgent([]string{"x"})
	r2 := b.reblind(r1)
	for i := range r1 {
		if r2[i].Cmp(r1[i]) == 0 {
			t.Fatalf("reblind[%d] unchanged — peer secret not applied", i)
		}
	}
}
