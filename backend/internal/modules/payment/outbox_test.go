package payment

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeOutbox is an in-memory OutboxRepository.
type fakeOutbox struct {
	due       []string
	enqueued  []string
	done      map[string]bool
	retries   map[string]int
	lastError map[string]string
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{done: map[string]bool{}, retries: map[string]int{}, lastError: map[string]string{}}
}

func (f *fakeOutbox) Enqueue(_ context.Context, orderID string) error {
	f.enqueued = append(f.enqueued, orderID)
	return nil
}
func (f *fakeOutbox) DueOrders(_ context.Context, _ int) ([]string, error) { return f.due, nil }
func (f *fakeOutbox) MarkDone(_ context.Context, orderID string) error {
	f.done[orderID] = true
	return nil
}
func (f *fakeOutbox) MarkRetry(_ context.Context, orderID, errMsg string, _, _ time.Duration, _ int) error {
	f.retries[orderID]++
	f.lastError[orderID] = errMsg
	return nil
}

// fakeLocker always grants the lock and runs fn inline (single-process test).
type fakeLocker struct{ acquired int }

func (l *fakeLocker) WithLock(ctx context.Context, _ string, fn func(context.Context) error) (bool, error) {
	l.acquired++
	return true, fn(ctx)
}

// neverLocker simulates another worker always holding the lock.
type neverLocker struct{}

func (neverLocker) WithLock(context.Context, string, func(context.Context) error) (bool, error) {
	return false, nil
}

func newOutboxSvc(o *fakeOrders, repo *fakeRepo, ob OutboxRepository, lk Locker) *Service {
	mock := MockProvider{Secret: secret}
	s := NewService(repo, o, mock, mock, nil)
	s.outbox = ob
	s.lock = lk
	return s
}

func TestSettleEnqueuesToOutbox(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", Status: "confirmed", PlatformFeeCents: 10000, SellerAmountCents: 90000}}
	repo := newFakeRepo()
	ob := newFakeOutbox()
	svc := newOutboxSvc(o, repo, ob, &fakeLocker{})

	if err := svc.Settle(ctx, "o1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(ob.enqueued) != 1 || ob.enqueued[0] != "o1" {
		t.Fatalf("expected o1 enqueued, got %v", ob.enqueued)
	}
	// Inline attempt also settled it.
	if o.settleCalls != 1 {
		t.Fatalf("MarkSettled called %d times, want 1", o.settleCalls)
	}
}

func TestDrainOutboxSettlesAndMarksDone(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", Status: "confirmed", PlatformFeeCents: 10000, SellerAmountCents: 90000}}
	repo := newFakeRepo()
	ob := newFakeOutbox()
	ob.due = []string{"o1"}
	lk := &fakeLocker{}
	svc := newOutboxSvc(o, repo, ob, lk)

	svc.drainSettlementOutbox(ctx)

	if !ob.done["o1"] {
		t.Fatal("expected o1 marked done")
	}
	if ob.retries["o1"] != 0 {
		t.Fatalf("expected no retries, got %d", ob.retries["o1"])
	}
	if lk.acquired != 1 {
		t.Fatalf("expected 1 lock acquisition, got %d", lk.acquired)
	}
}

func TestDrainOutboxRetriesOnFailure(t *testing.T) {
	ctx := context.Background()
	// Order not confirmed -> settleOnce returns ErrNotConfirmed -> retry recorded.
	o := &fakeOrders{info: OrderInfo{ID: "o9", Status: "paid"}}
	repo := newFakeRepo()
	ob := newFakeOutbox()
	ob.due = []string{"o9"}
	svc := newOutboxSvc(o, repo, ob, &fakeLocker{})

	svc.drainSettlementOutbox(ctx)

	if ob.done["o9"] {
		t.Fatal("must not mark a failed settlement done")
	}
	if ob.retries["o9"] != 1 {
		t.Fatalf("expected 1 retry recorded, got %d", ob.retries["o9"])
	}
	if !strings.Contains(ob.lastError["o9"], "not confirmed") {
		t.Fatalf("expected not-confirmed error recorded, got %q", ob.lastError["o9"])
	}
}

func TestDrainOutboxSkipsWhenLockHeld(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", Status: "confirmed", PlatformFeeCents: 1, SellerAmountCents: 1}}
	repo := newFakeRepo()
	ob := newFakeOutbox()
	ob.due = []string{"o1"}
	svc := newOutboxSvc(o, repo, ob, neverLocker{})

	svc.drainSettlementOutbox(ctx)

	if ob.done["o1"] || ob.retries["o1"] != 0 {
		t.Fatal("a lock held by another worker must leave the job untouched")
	}
	if o.settleCalls != 0 {
		t.Fatal("must not settle when the lock is not acquired")
	}
}
