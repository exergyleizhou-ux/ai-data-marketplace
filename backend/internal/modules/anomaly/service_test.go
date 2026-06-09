package anomaly

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeRule struct {
	kind    string
	output  []Anomaly
	callErr error
}

func (r *fakeRule) Kind() string { return r.kind }
func (r *fakeRule) Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error) {
	return r.output, r.callErr
}

type fakeAnomalyRepo struct {
	mu       sync.Mutex
	upserted []Anomaly
	err      error
}

func (r *fakeAnomalyRepo) Upsert(_ context.Context, a Anomaly) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, a)
	return r.err
}
func (r *fakeAnomalyRepo) List(_ context.Context, _ string, _, _ int) ([]Anomaly, error) {
	return nil, nil
}
func (r *fakeAnomalyRepo) Get(_ context.Context, _ string) (Anomaly, error) { return Anomaly{}, nil }
func (r *fakeAnomalyRepo) SetStatus(_ context.Context, _, _, _, _ string) (Anomaly, error) {
	return Anomaly{}, nil
}

func TestScanOnce_RunsAllRulesAndUpsertsResults(t *testing.T) {
	repo := &fakeAnomalyRepo{}
	svc := &Service{
		repo: repo,
		rules: []Rule{
			&fakeRule{kind: "r1", output: []Anomaly{{Kind: "r1", ResourcePattern: "x"}}},
			&fakeRule{kind: "r2", output: []Anomaly{{Kind: "r2", ResourcePattern: "y"}}},
		},
	}
	n, err := svc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("total = %d, want 2", n)
	}
	if len(repo.upserted) != 2 {
		t.Fatalf("upserted = %d, want 2", len(repo.upserted))
	}
}

func TestScanOnce_RuleFailureDoesNotBlockOthers(t *testing.T) {
	repo := &fakeAnomalyRepo{}
	svc := &Service{
		repo: repo,
		rules: []Rule{
			&fakeRule{kind: "r1", callErr: context.DeadlineExceeded},
			&fakeRule{kind: "r2", output: []Anomaly{{Kind: "r2", ResourcePattern: "ok"}}},
		},
	}
	n, err := svc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("r1 failure should not block r2, total = %d, want 1", n)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("upserted = %d, want 1", len(repo.upserted))
	}
}

func TestUpsert_DoesNotOverrideResolvedAnomaly(t *testing.T) {
	// The SQL has WHERE status='open' in the ON CONFLICT DO UPDATE clause.
	// This test verifies the documented guarantee; the SQL verification
	// happens in the real-DB repo_test.
	t.Skip("ON CONFLICT DO UPDATE WHERE status='open' is a SQL-level guarantee; covered by repo test")
}
