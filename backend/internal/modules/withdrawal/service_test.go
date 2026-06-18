package withdrawal

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// --- fakes ---

type fakeRepo struct {
	reqs map[string]Request
}

func (r *fakeRepo) Create(_ context.Context, req Request) (Request, error) {
	req.ID = "wd-" + req.Channel
	req.Status = StatusPending
	if r.reqs == nil {
		r.reqs = map[string]Request{}
	}
	r.reqs[req.ID] = req
	return req, nil
}
func (r *fakeRepo) Get(_ context.Context, id string) (Request, error) { return Request{}, ErrNotFound }
func (r *fakeRepo) ListBySeller(_ context.Context, _ string, _, _ int) ([]Request, error) {
	return nil, nil
}
func (r *fakeRepo) AdminList(_ context.Context, _ string, _, _ int) ([]Request, error) {
	return nil, nil
}
func (r *fakeRepo) Transition(_ context.Context, id, from, to, opsID, note string) (Request, error) {
	if from == StatusCompleted {
		return Request{}, ErrBadTransition
	}
	rr, ok := r.reqs[id]
	if !ok {
		return Request{}, ErrNotFound
	}
	if rr.Status != from {
		return Request{}, ErrBadTransition
	}
	rr.Status = to
	rr.ProcessedBy = opsID
	rr.OpsNote = note
	r.reqs[id] = rr
	return rr, nil
}
func (r *fakeRepo) SumApprovedAndPending(_ context.Context, _ string) (int64, error) {
	return 500, nil // default: 500 pending
}
func (r *fakeRepo) CreateWithinBudget(_ context.Context, req Request, settledCents int64) (Request, error) {
	if r.reqs == nil {
		r.reqs = map[string]Request{}
	}
	var outstanding int64
	for _, x := range r.reqs {
		if x.SellerID == req.SellerID && x.Status != StatusRejected {
			outstanding += x.AmountCents // pending + approved + completed all consume the balance
		}
	}
	if req.AmountCents > settledCents-outstanding {
		return Request{}, ErrInsufficientBalance
	}
	req.ID = fmt.Sprintf("wd-%d", len(r.reqs))
	req.Status = StatusPending
	r.reqs[req.ID] = req
	return req, nil
}

type fakeEarnings struct{ settled int64 }

func (f *fakeEarnings) SettledCentsOf(_ context.Context, _ string) (int64, error) {
	return f.settled, nil
}

type fakeWDNotifier struct {
	mu    sync.Mutex
	calls []wdNotifyCall
}
type wdNotifyCall struct{ UserID, Kind string }

func (f *fakeWDNotifier) NotifyUser(_ context.Context, userID, kind, _, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, wdNotifyCall{userID, kind})
	return nil
}

// --- tests ---

func TestRequest_RejectsAmountExceedingAvailable(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeEarnings{settled: 500}, nil)
	_, err := svc.Request(context.Background(), "s1", 700, "bank", "label")
	if err != ErrInsufficientBalance {
		t.Fatalf("available=500, request=700 must exceed, got %v", err)
	}
}

func TestRequest_AcceptsAmountAtAvailable(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeEarnings{settled: 500}, nil)
	_, err := svc.Request(context.Background(), "s1", 500, "bank", "label")
	if err != nil {
		t.Fatalf("request at available must succeed, got %v", err)
	}
}

func TestRequest_RejectsInvalidChannel(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeEarnings{settled: 10000}, nil)
	_, err := svc.Request(context.Background(), "s1", 100, "bitcoin", "l")
	if err != ErrChannelInvalid {
		t.Fatalf("want ErrChannelInvalid, got %v", err)
	}
}

func TestRequest_RejectsZeroOrNegativeAmount(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeEarnings{settled: 10000}, nil)
	_, err := svc.Request(context.Background(), "s1", 0, "bank", "l")
	if err != ErrAmountInvalid {
		t.Fatalf("zero amount must be invalid, got %v", err)
	}
}

func TestApprove_NotifiesSellerAndNotOps(t *testing.T) {
	repo := &fakeRepo{}
	r, _ := repo.Create(context.Background(), Request{SellerID: "seller", AmountCents: 500, Channel: "bank", AccountLabel: "a"})
	repo.reqs[r.ID] = r

	notifier := &fakeWDNotifier{}
	svc := NewService(repo, &fakeEarnings{}, notifier)
	_, err := svc.Approve(context.Background(), "ops-user", r.ID, "approved")
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(notifier.calls))
	}
	if notifier.calls[0].UserID != "seller" {
		t.Fatalf("notified user = %q, want seller (not ops)", notifier.calls[0].UserID)
	}
	if notifier.calls[0].Kind != "withdrawal_approved" {
		t.Fatalf("kind = %q, want withdrawal_approved", notifier.calls[0].Kind)
	}
}

// Regression: a COMPLETED payout must keep consuming the balance. The old code
// subtracted only pending+approved, so a paid-out withdrawal freed the balance and
// let the seller re-withdraw the same earnings indefinitely.
func TestRequest_CompletedWithdrawalsReduceAvailable(t *testing.T) {
	repo := &fakeRepo{reqs: map[string]Request{
		"old": {ID: "old", SellerID: "s1", AmountCents: 1000, Status: StatusCompleted},
	}}
	svc := NewService(repo, &fakeEarnings{settled: 1000}, nil)
	// The full 1000 settled has already been paid out → available is 0.
	if _, err := svc.Request(context.Background(), "s1", 1, "bank", "label"); err != ErrInsufficientBalance {
		t.Fatalf("a completed payout must keep consuming the balance; got %v", err)
	}
}
