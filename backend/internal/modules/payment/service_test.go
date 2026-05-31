package payment

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	paid       map[string]bool // channelTxn -> already paid
	txnToOrder map[string]string
	settled    map[string]bool // orderID -> settlement exists
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{paid: map[string]bool{}, txnToOrder: map[string]string{}, settled: map[string]bool{}}
}

func (r *fakeRepo) EnsurePayment(_ context.Context, orderID, _, channelTxnID string, _ int64) error {
	r.txnToOrder[channelTxnID] = orderID
	return nil
}
func (r *fakeRepo) MarkPaidByChannelTxn(_ context.Context, channelTxnID string) (string, bool, error) {
	orderID, ok := r.txnToOrder[channelTxnID]
	if !ok {
		return "", false, errors.New("unknown txn")
	}
	if r.paid[channelTxnID] {
		return orderID, false, nil // already handled
	}
	r.paid[channelTxnID] = true
	return orderID, true, nil
}
func (r *fakeRepo) CreateSettlement(_ context.Context, orderID, _ string, _, _ int64) (bool, error) {
	if r.settled[orderID] {
		return false, nil
	}
	r.settled[orderID] = true
	return true, nil
}
func (r *fakeRepo) MarkSettlementSuccess(_ context.Context, _, _ string) error { return nil }
func (r *fakeRepo) ChannelTxnByOrder(_ context.Context, orderID string) (string, error) {
	for txn, oid := range r.txnToOrder {
		if oid == orderID {
			return txn, nil
		}
	}
	return "", errors.New("no payment")
}

type fakeOrders struct {
	info        OrderInfo
	paidCalls   int
	settleCalls int
}

func (f *fakeOrders) GetSystem(_ context.Context, _ string) (OrderInfo, error) { return f.info, nil }
func (f *fakeOrders) MarkPaid(_ context.Context, _ string) error {
	f.paidCalls++
	f.info.Status = "paid"
	return nil
}
func (f *fakeOrders) MarkSettled(_ context.Context, _ string) error {
	f.settleCalls++
	f.info.Status = "settled"
	return nil
}

const secret = "s3cr3t"

func newSvc(o *fakeOrders) (*Service, *fakeRepo) {
	repo := newFakeRepo()
	mock := MockProvider{Secret: secret}
	return NewService(repo, o, mock, mock, nil), repo
}

func TestCreatePaymentGuards(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", BuyerID: "buyer", Status: "created", AmountCents: 100000}}
	svc, _ := newSvc(o)

	if _, err := svc.CreatePayment(ctx, "intruder", "o1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	o.info.Status = "paid"
	if _, err := svc.CreatePayment(ctx, "buyer", "o1"); !errors.Is(err, ErrOrderNotPayable) {
		t.Fatalf("want ErrOrderNotPayable, got %v", err)
	}
}

func TestCallbackIdempotentAndSigned(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", BuyerID: "buyer", Status: "created", AmountCents: 100000}}
	svc, _ := newSvc(o)

	info, err := svc.CreatePayment(ctx, "buyer", "o1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	payload := "o1:" + info.ChannelTxnID + ":true"
	goodSig := Sign(secret, payload)

	// Bad signature rejected.
	if err := svc.HandleCallback(ctx, "mock", []byte(payload), "deadbeef"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}

	// First valid callback marks paid once.
	if err := svc.HandleCallback(ctx, "mock", []byte(payload), goodSig); err != nil {
		t.Fatalf("callback: %v", err)
	}
	// Duplicate callback is a no-op (idempotent).
	if err := svc.HandleCallback(ctx, "mock", []byte(payload), goodSig); err != nil {
		t.Fatalf("duplicate callback: %v", err)
	}
	if o.paidCalls != 1 {
		t.Fatalf("order MarkPaid called %d times, want exactly 1", o.paidCalls)
	}
}

func TestSettleIdempotent(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", Status: "confirmed", PlatformFeeCents: 10000, SellerAmountCents: 90000}}
	svc, _ := newSvc(o)

	// Not-confirmed guard.
	notConfirmed := &fakeOrders{info: OrderInfo{ID: "o2", Status: "paid"}}
	svc2, _ := newSvc(notConfirmed)
	if err := svc2.Settle(ctx, "o2"); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("want ErrNotConfirmed, got %v", err)
	}

	if err := svc.Settle(ctx, "o1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	// Simulate a retry that races in before the order status flips: the
	// settlement row already exists, so CreateSettlement returns false and we
	// must NOT split/settle again (double-split guard).
	o.info.Status = "confirmed"
	if err := svc.Settle(ctx, "o1"); err != nil {
		t.Fatalf("re-settle: %v", err)
	}
	if o.settleCalls != 1 {
		t.Fatalf("MarkSettled called %d times, want exactly 1", o.settleCalls)
	}
}
