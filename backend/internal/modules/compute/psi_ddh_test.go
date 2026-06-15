package compute

import (
	"context"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

// TestDDHPSIMatchesMockSemantics is the correctness anchor: real DDH-PSI must
// return the EXACT same intersection as the (trivially correct) in-process mock,
// across many randomized inputs — but computed cryptographically.
func TestDDHPSIMatchesMockSemantics(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	mock := NewMockMPC()
	ddh := NewDDHPSI()
	for trial := 0; trial < 25; trial++ {
		nParties := 2 + rng.Intn(3) // 2..4 parties
		parties := make([][]string, nParties)
		for i := range parties {
			n := rng.Intn(8)
			set := make([]string, n)
			for j := range set {
				set[j] = "e" + strconv.Itoa(rng.Intn(12)) // overlap by drawing from a small universe
			}
			parties[i] = set
		}
		want, err := mock.RunPSI(context.Background(), parties)
		if err != nil {
			t.Fatalf("mock: %v", err)
		}
		got, err := ddh.RunPSI(context.Background(), parties)
		if err != nil {
			t.Fatalf("ddh: %v", err)
		}
		if !reflect.DeepEqual(got.Intersection, want.Intersection) {
			t.Fatalf("trial %d: ddh=%v want=%v (parties=%v)", trial, got.Intersection, want.Intersection, parties)
		}
		if got.Cardinality != want.Cardinality {
			t.Fatalf("trial %d: cardinality ddh=%d want=%d", trial, got.Cardinality, want.Cardinality)
		}
	}
}

// TestDDHBlindingCommutesAndHidesPlaintext pins the two cryptographic facts the
// protocol rests on:
//  1. blinding commutes: applying party A then B's secret yields the same group
//     element as B then A — this is what lets the orchestrator match a shared
//     element across parties WITHOUT ever comparing plaintext;
//  2. a blinded element is not its plaintext and differs per secret — the
//     orchestrator that only ever sees blinded points learns nothing trivially.
func TestDDHBlindingCommutesAndHidesPlaintext(t *testing.T) {
	a, err := newPSISecret()
	if err != nil {
		t.Fatalf("secret a: %v", err)
	}
	b, err := newPSISecret()
	if err != nil {
		t.Fatalf("secret b: %v", err)
	}
	pt := psiHashToGroup("alice@example.com")

	ab := psiBlind(psiBlind(pt, a), b)
	ba := psiBlind(psiBlind(pt, b), a)
	if ab.Cmp(ba) != 0 {
		t.Fatal("blinding must commute: H(x)^{ab} != H(x)^{ba}")
	}
	// A different element must not collide with this one's double-blinding.
	other := psiBlind(psiBlind(psiHashToGroup("bob@example.com"), a), b)
	if other.Cmp(ab) == 0 {
		t.Fatal("distinct elements collided under double-blinding")
	}
	// The blinded point must not equal the bare hash-to-group point (it is masked).
	if ab.Cmp(pt) == 0 {
		t.Fatal("blinded point equals the unblinded point — no masking applied")
	}
}

func TestDDHPSIRejectsSingleParty(t *testing.T) {
	if _, err := NewDDHPSI().RunPSI(context.Background(), [][]string{{"x"}}); err != ErrMPCParties {
		t.Fatalf("single party must return ErrMPCParties, got %v", err)
	}
}

func TestDDHPSIKind(t *testing.T) {
	if NewDDHPSI().Kind() != "ddh-psi" {
		t.Fatalf("kind = %q", NewDDHPSI().Kind())
	}
}
