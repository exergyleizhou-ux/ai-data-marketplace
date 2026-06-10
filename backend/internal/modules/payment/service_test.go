package payment

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	payURLs      map[string]string // orderID -> stored pay url
	paid         map[string]bool   // channelTxn -> already paid
	txnToOrder   map[string]string
	settleStatus map[string]string // orderID -> settlement status (pending/success/reverted)
	splitTxn     map[string]string // orderID -> split txn id (settled successfully)
	refunded     map[string]bool   // orderID -> refund recorded
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{paid: map[string]bool{}, txnToOrder: map[string]string{}, settleStatus: map[string]string{},
		splitTxn: map[string]string{}, refunded: map[string]bool{}}
}

func (r *fakeRepo) SettlementState(_ context.Context, orderID string) (string, bool, error) {
	st, ok := r.settleStatus[orderID]
	return st, ok, nil
}

func (r *fakeRepo) RefundContext(_ context.Context, orderID string) (string, string, error) {
	var channelTxn string
	for txn, oid := range r.txnToOrder {
		if oid == orderID {
			channelTxn = txn
		}
	}
	if channelTxn == "" {
		return "", "", errors.New("no payment for order")
	}
	return channelTxn, r.splitTxn[orderID], nil
}

func (r *fakeRepo) MarkRefunded(_ context.Context, orderID string) error {
	r.refunded[orderID] = true
	return nil
}

func (r *fakeRepo) EnsurePayment(_ context.Context, orderID, _, channelTxnID, payURL string, _ int64) (string, string, error) {
	// Mirror the SQL semantics: first insert wins, later calls get the winner.
	for txn, oid := range r.txnToOrder {
		if oid == orderID {
			return txn, r.payURLs[oid], nil
		}
	}
	r.txnToOrder[channelTxnID] = orderID
	if r.payURLs == nil {
		r.payURLs = map[string]string{}
	}
	r.payURLs[orderID] = payURL
	return channelTxnID, payURL, nil
}

func (r *fakeRepo) PaymentForOrder(_ context.Context, orderID string) (string, string, string, bool, error) {
	for txn, oid := range r.txnToOrder {
		if oid == orderID {
			status := "created"
			if r.paid[txn] {
				status = "paid"
			}
			return txn, r.payURLs[oid], status, true, nil
		}
	}
	return "", "", "", false, nil
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
	if _, ok := r.settleStatus[orderID]; ok {
		return false, nil
	}
	r.settleStatus[orderID] = "pending"
	return true, nil
}
func (r *fakeRepo) MarkSettlementSuccess(_ context.Context, orderID, splitTxnID string) error {
	r.settleStatus[orderID] = "success"
	r.splitTxn[orderID] = splitTxnID
	return nil
}
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

// recordingRefund wraps MockProvider to capture the args passed to Refund.
type recordingRefund struct {
	MockProvider
	gotChannelTxn string
	gotSplitTxn   string
	gotAmount     int64
	calls         int
}

func (r *recordingRefund) Refund(ctx context.Context, channelTxn, splitTxn string, amount int64) (string, error) {
	r.calls++
	r.gotChannelTxn, r.gotSplitTxn, r.gotAmount = channelTxn, splitTxn, amount
	return r.MockProvider.Refund(ctx, channelTxn, splitTxn, amount)
}

func TestRefundReversesSettledOrder(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", BuyerID: "buyer", Status: "confirmed", AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000}}
	repo := newFakeRepo()
	rp := &recordingRefund{MockProvider: MockProvider{Secret: secret}}
	svc := NewService(repo, o, rp, rp, nil)

	// Drive a settlement so a split txn exists to reverse later.
	repo.txnToOrder["pi-o1"] = "o1" // a known channel txn for the order
	if err := svc.Settle(ctx, "o1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if repo.splitTxn["o1"] == "" {
		t.Fatal("expected a split txn after settle")
	}

	// Now refund: provider must receive the split txn (to reverse) + amount.
	if err := svc.Refund(ctx, "o1"); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if rp.calls != 1 {
		t.Fatalf("Refund called %d times, want 1", rp.calls)
	}
	if rp.gotSplitTxn != repo.splitTxn["o1"] {
		t.Fatalf("refund got split txn %q, want %q", rp.gotSplitTxn, repo.splitTxn["o1"])
	}
	if rp.gotAmount != 100000 {
		t.Fatalf("refund amount %d, want 100000", rp.gotAmount)
	}
	if !repo.refunded["o1"] {
		t.Fatal("expected payment marked refunded")
	}
}

func TestRefundUnsettledHasNoSplitToReverse(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o2", BuyerID: "buyer", Status: "paid", AmountCents: 50000}}
	repo := newFakeRepo()
	rp := &recordingRefund{MockProvider: MockProvider{Secret: secret}}
	svc := NewService(repo, o, rp, rp, nil)
	repo.txnToOrder["pi-o2"] = "o2"

	if err := svc.Refund(ctx, "o2"); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if rp.gotSplitTxn != "" {
		t.Fatalf("expected no split txn to reverse, got %q", rp.gotSplitTxn)
	}
	if !repo.refunded["o2"] {
		t.Fatal("expected payment marked refunded")
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
