package payment

import (
	"context"
	"errors"
	"testing"
)

// countingProvider wraps MockProvider but mints a DIFFERENT txn per call —
// like Stripe (a new PaymentIntent per create). The deterministic MockProvider
// masked the retry bug: same order always produced the same txn.
type countingProvider struct {
	MockProvider
	calls int
}

func (p *countingProvider) CreatePayment(orderID string, _ int64) (CreateResult, error) {
	p.calls++
	txn := Sign(p.Secret, orderID)[:8] + "-" + string(rune('a'+p.calls-1))
	return CreateResult{ChannelTxnID: txn, PayURL: "https://pay.example/" + txn}, nil
}

// A retried POST /payments must return the SAME charge as the first call and
// must NOT mint a second provider charge. (Real-money bug: the DB keeps the
// first txn, the buyer pays the second — the webhook never matches.)
func TestCreatePayment_RetryReturnsSameCharge(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o1", BuyerID: "buyer", Status: "created", AmountCents: 100000}}
	repo := newFakeRepo()
	prov := &countingProvider{MockProvider: MockProvider{Secret: secret}}
	svc := NewService(repo, o, prov, MockProvider{Secret: secret}, nil)

	first, err := svc.CreatePayment(ctx, "buyer", "o1")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := svc.CreatePayment(ctx, "buyer", "o1")
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if second.ChannelTxnID != first.ChannelTxnID || second.PayURL != first.PayURL {
		t.Fatalf("retry diverged: first=%+v second=%+v", first, second)
	}
	if prov.calls != 1 {
		t.Fatalf("provider charged %d times, want 1", prov.calls)
	}
}

// Under a concurrent double-create both callers reach the provider, but the
// response must carry the row that WON the insert — the only txn the webhook
// can ever match.
func TestCreatePayment_ConcurrentLoserGetsWinnerRow(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o2", BuyerID: "buyer", Status: "created", AmountCents: 5000}}
	repo := newFakeRepo()
	// Simulate the race: the winner's row is already in the repo, but
	// PaymentForOrder hasn't seen it (loser passed the existence check first).
	repo.txnToOrder["winner-txn"] = "o2"
	repo.payURLs = map[string]string{"o2": "https://pay.example/winner"}
	prov := &countingProvider{MockProvider: MockProvider{Secret: secret}}
	svc := NewService(repo, o, prov, MockProvider{Secret: secret}, nil)

	got, err := svc.CreatePayment(ctx, "buyer", "o2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ChannelTxnID != "winner-txn" || got.PayURL != "https://pay.example/winner" {
		t.Fatalf("must return winner row, got %+v", got)
	}
	// And the repo-level conflict semantics: a direct EnsurePayment with a
	// losing charge returns the winner.
	winTxn, winURL, err := repo.EnsurePayment(ctx, "o2", "mock", "loser-txn", "https://pay.example/loser", 5000)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if winTxn != "winner-txn" || winURL != "https://pay.example/winner" {
		t.Fatalf("EnsurePayment must return winner, got %s %s", winTxn, winURL)
	}
}

// A payment already paid must refuse a new charge even if the order status
// lagged (defense in depth on the payment row itself).
func TestCreatePayment_PaidPaymentNotPayable(t *testing.T) {
	ctx := context.Background()
	o := &fakeOrders{info: OrderInfo{ID: "o3", BuyerID: "buyer", Status: "created", AmountCents: 100}}
	repo := newFakeRepo()
	repo.txnToOrder["txn3"] = "o3"
	repo.paid["txn3"] = true
	svc := NewService(repo, o, MockProvider{Secret: secret}, MockProvider{Secret: secret}, nil)

	if _, err := svc.CreatePayment(ctx, "buyer", "o3"); !errors.Is(err, ErrOrderNotPayable) {
		t.Fatalf("want ErrOrderNotPayable, got %v", err)
	}
}
