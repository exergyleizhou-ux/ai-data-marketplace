package compute

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMockMPC_PSIIntersection(t *testing.T) {
	// Three parties; only "b" and "d" appear in ALL three.
	parties := [][]string{
		{"a", "b", "c", "d"},
		{"b", "d", "e"},
		{"b", "d", "x", "y"},
	}
	res, err := NewMockMPC().RunPSI(context.Background(), parties)
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	want := []string{"b", "d"} // sorted, deduped
	if !reflect.DeepEqual(res.Intersection, want) {
		t.Fatalf("intersection = %v, want %v", res.Intersection, want)
	}
	if res.Cardinality != 2 {
		t.Fatalf("cardinality = %d, want 2", res.Cardinality)
	}
}

func TestMockMPC_PSIEmptyWhenDisjoint(t *testing.T) {
	res, err := NewMockMPC().RunPSI(context.Background(), [][]string{{"a", "b"}, {"c", "d"}})
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	if res.Cardinality != 0 || len(res.Intersection) != 0 {
		t.Fatalf("disjoint sets must yield an empty intersection, got %v", res.Intersection)
	}
}

func TestMockMPC_PSIDedupesWithinParty(t *testing.T) {
	// "b" repeated within a party must not inflate the intersection.
	parties := [][]string{
		{"a", "b", "b", "b"},
		{"b", "b", "c"},
	}
	res, err := NewMockMPC().RunPSI(context.Background(), parties)
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	if !reflect.DeepEqual(res.Intersection, []string{"b"}) {
		t.Fatalf("intersection = %v, want [b]", res.Intersection)
	}
}

func TestMockMPC_PSIDeterministicSortedOrder(t *testing.T) {
	// Unsorted, overlapping inputs must produce a sorted, stable result.
	parties := [][]string{
		{"delta", "alpha", "charlie", "bravo"},
		{"bravo", "charlie", "alpha", "delta"},
	}
	res, err := NewMockMPC().RunPSI(context.Background(), parties)
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie", "delta"}
	if !reflect.DeepEqual(res.Intersection, want) {
		t.Fatalf("intersection = %v, want %v (sorted)", res.Intersection, want)
	}
}

func TestMockMPC_PSIRequiresTwoParties(t *testing.T) {
	if _, err := NewMockMPC().RunPSI(context.Background(), [][]string{{"a"}}); !errors.Is(err, ErrMPCParties) {
		t.Fatalf("a single party must be refused with ErrMPCParties, got %v", err)
	}
	if _, err := NewMockMPC().RunPSI(context.Background(), nil); !errors.Is(err, ErrMPCParties) {
		t.Fatalf("nil parties must be refused with ErrMPCParties")
	}
}

func TestMockMPC_PSIEmptyPartyYieldsEmptyIntersection(t *testing.T) {
	// A party contributing no elements can share nothing → empty intersection.
	res, err := NewMockMPC().RunPSI(context.Background(), [][]string{{"a", "b"}, {}})
	if err != nil {
		t.Fatalf("psi: %v", err)
	}
	if res.Cardinality != 0 {
		t.Fatalf("an empty party must force an empty intersection, got %v", res.Intersection)
	}
}

func TestMockMPC_Kind(t *testing.T) {
	if NewMockMPC().Kind() != "mock" {
		t.Fatalf("kind = %q", NewMockMPC().Kind())
	}
}
