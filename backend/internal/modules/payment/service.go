package payment

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// OrderInfo is the order data the payment module needs (provided by the order
// module via OrderGateway, so payment never imports order internals).
type OrderInfo struct {
	ID                string
	BuyerID           string
	SellerID          string
	Status            string
	AmountCents       int64
	PlatformFeeCents  int64
	SellerAmountCents int64
}

// OrderGateway lets payment read an order and drive its money transitions.
type OrderGateway interface {
	GetSystem(ctx context.Context, orderID string) (OrderInfo, error)
	MarkPaid(ctx context.Context, orderID string) error
	MarkSettled(ctx context.Context, orderID string) error
}

// Service orchestrates payment + split-settlement against a licensed provider.
type Service struct {
	repo     Repository
	orders   OrderGateway
	provider PaymentProvider
	split    SplitProvider
	refund   RefundProvider // optional; set when the provider supports refunds (H2)
	audit    audit.Recorder

	// Settlement outbox (H3): durable retry of failed settlements. nil unless
	// StartSettlementOutbox was called (server with a DB).
	outbox       OutboxRepository
	lock         Locker
	outboxStop   chan struct{}
	outboxWG     sync.WaitGroup
	outboxTicker time.Duration
}

// Settlement-outbox tuning. Kept small/conservative for the MVP; a real
// deployment would read these from config.
const (
	outboxBatch         = 50
	maxSettleAttempts   = 6
	settleBackoffBase   = 30 * time.Second
	settleBackoffCap    = 30 * time.Minute
	defaultOutboxTicker = 15 * time.Second
)

func NewService(repo Repository, orders OrderGateway, provider PaymentProvider, split SplitProvider, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	s := &Service{repo: repo, orders: orders, provider: provider, split: split, audit: rec}
	// Both the mock and Stripe providers implement RefundProvider; wire it when
	// available so dispute refunds can move real money (H2).
	if rp, ok := provider.(RefundProvider); ok {
		s.refund = rp
	}
	return s
}

// CreatePayment initiates a charge for the buyer's created order and returns the
// provider pay URL. Funds will be escrowed at the provider, never on-platform.
func (s *Service) CreatePayment(ctx context.Context, buyerID, orderID string) (PayInfo, error) {
	o, err := s.orders.GetSystem(ctx, orderID)
	if err != nil {
		return PayInfo{}, err
	}
	if o.BuyerID != buyerID {
		return PayInfo{}, ErrForbidden
	}
	if o.Status != "created" {
		return PayInfo{}, ErrOrderNotPayable
	}
	// Idempotent create: a retried POST /payments must return the SAME charge.
	// Minting a new provider charge per retry made the buyer pay a txn the DB
	// row doesn't carry — the webhook then never matches and the order sticks.
	if txn, url, status, found, err := s.repo.PaymentForOrder(ctx, orderID); err != nil {
		return PayInfo{}, err
	} else if found {
		if status != "created" {
			return PayInfo{}, ErrOrderNotPayable
		}
		return PayInfo{OrderID: orderID, ChannelTxnID: txn, PayURL: url, AmountCents: o.AmountCents, Channel: s.provider.Channel()}, nil
	}
	res, err := s.provider.CreatePayment(orderID, o.AmountCents)
	if err != nil {
		return PayInfo{}, fmt.Errorf("provider create payment: %w", err)
	}
	// Respond with the WINNING row — under a concurrent double-create the
	// loser's provider charge is orphaned (harmless, never confirmable by us)
	// and both callers see the txn the webhook will actually match.
	winTxn, winURL, err := s.repo.EnsurePayment(ctx, orderID, s.provider.Channel(), res.ChannelTxnID, res.PayURL, o.AmountCents)
	if err != nil {
		return PayInfo{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "payment.create", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"channel": s.provider.Channel(), "channel_txn_id": winTxn}})
	return PayInfo{OrderID: orderID, ChannelTxnID: winTxn, PayURL: winURL, AmountCents: o.AmountCents, Channel: s.provider.Channel()}, nil
}

// HandleCallback processes a provider webhook: verify signature, then mark the
// payment paid and the order paid — exactly once (idempotent on channel_txn_id).
func (s *Service) HandleCallback(ctx context.Context, channel string, payload []byte, signature string) error {
	if channel != s.provider.Channel() {
		return fmt.Errorf("unsupported channel %q", channel)
	}
	cb, err := s.provider.VerifyCallback(payload, signature)
	if err != nil {
		return err
	}
	if !cb.Paid {
		return nil // non-success notification; nothing to do
	}
	return s.markPaid(ctx, cb.ChannelTxnID)
}

// markPaid flips the payment + order to paid exactly once (idempotent on
// channel_txn_id). Shared by the verified webhook and the dev helper.
func (s *Service) markPaid(ctx context.Context, channelTxnID string) error {
	orderID, newlyPaid, err := s.repo.MarkPaidByChannelTxn(ctx, channelTxnID)
	if err != nil {
		return err
	}
	if !newlyPaid {
		return nil // already handled
	}
	if err := s.orders.MarkPaid(ctx, orderID); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Entry{Action: "payment.paid", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"channel_txn_id": channelTxnID}})
	return nil
}

// DevMarkPaid simulates a successful provider callback for an order. SANDBOX/DEV
// ONLY — it bypasses signature verification and must never be mounted in
// production. It lets the UI demo the full loop without a real payment gateway.
func (s *Service) DevMarkPaid(ctx context.Context, orderID string) error {
	txn, err := s.repo.ChannelTxnByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	return s.markPaid(ctx, txn)
}

// Refund reverses a payment when ops resolves a dispute for the buyer (H2): it
// reverses the seller transfer if the order had already settled, refunds the
// buyer's charge at the provider, then flips the payment to refunded and any
// settlement to reverted. The provider call runs first so a provider failure
// leaves the order untouched (the caller keeps the dispute open for retry).
func (s *Service) Refund(ctx context.Context, orderID string) error {
	if s.refund == nil {
		return ErrRefundUnsupported
	}
	o, err := s.orders.GetSystem(ctx, orderID)
	if err != nil {
		return err
	}
	channelTxn, splitTxn, err := s.repo.RefundContext(ctx, orderID)
	if err != nil {
		return err
	}
	refundTxnID, err := s.refund.Refund(ctx, channelTxn, splitTxn, o.AmountCents)
	if err != nil {
		return fmt.Errorf("provider refund: %w", err)
	}
	if err := s.repo.MarkRefunded(ctx, orderID); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Entry{Action: "payment.refund", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"refund_txn_id": refundTxnID, "split_txn_id": splitTxn, "amount_cents": o.AmountCents}})
	return nil
}

// Settle is the settlement entrypoint, called when an order is confirmed. With
// the outbox enabled (H3) it first records a durable job, then attempts an
// inline settlement; if the inline attempt fails the durable row guarantees a
// background worker retries it. Without the outbox it just settles once
// (previous behaviour). Either way the work is idempotent.
func (s *Service) Settle(ctx context.Context, orderID string) error {
	if s.outbox != nil {
		if err := s.outbox.Enqueue(ctx, orderID); err != nil {
			return fmt.Errorf("enqueue settlement: %w", err)
		}
	}
	return s.settleOnce(ctx, orderID)
}

// settleOnce performs (or resumes) one split-settlement: seller share +
// platform commission move out of escrow at the provider, then the order
// becomes settled. It is idempotent and retriable:
//   - if the settlement already succeeded/reverted, it's a no-op;
//   - if a pending settlement row exists (a prior attempt created it but the
//     provider call didn't finish), it re-executes the split — safe because the
//     provider call carries an idempotency key (no double transfer);
//   - the settlements unique key remains the double-split guard (docs §6.6).
func (s *Service) settleOnce(ctx context.Context, orderID string) error {
	status, exists, err := s.repo.SettlementState(ctx, orderID)
	if err != nil {
		return err
	}
	if exists && (status == SettleSuccess || status == SettleReverted) {
		return nil // already settled, or clawed back by a refund — done
	}

	o, err := s.orders.GetSystem(ctx, orderID)
	if err != nil {
		return err
	}
	if !exists {
		if o.Status != "confirmed" {
			return ErrNotConfirmed
		}
		if _, err := s.repo.CreateSettlement(ctx, orderID, "settle:"+orderID, o.PlatformFeeCents, o.SellerAmountCents); err != nil {
			return err
		}
		// created==false here means a concurrent attempt inserted it first; we
		// still fall through to execute the now-pending row (idempotent split).
	}

	splitTxnID, err := s.split.ExecuteSplit(ctx, orderID, o.SellerID, o.SellerAmountCents, o.PlatformFeeCents)
	if err != nil {
		return fmt.Errorf("execute split: %w", err)
	}
	if err := s.repo.MarkSettlementSuccess(ctx, orderID, splitTxnID); err != nil {
		return err
	}
	if err := s.orders.MarkSettled(ctx, orderID); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Entry{Action: "settlement.success", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"split_txn_id": splitTxnID, "platform_fee_cents": o.PlatformFeeCents, "seller_amount_cents": o.SellerAmountCents}})
	return nil
}

// StartSettlementOutbox enables durable settlement (H3): Settle then records a
// job, and a background worker drains due jobs under a distributed lock,
// retrying with backoff. Call Close on shutdown to stop the worker. Safe to
// call once; a second call is ignored.
func (s *Service) StartSettlementOutbox(repo OutboxRepository, lock Locker) {
	if s.outbox != nil || repo == nil {
		return
	}
	s.outbox = repo
	s.lock = lock
	s.outboxStop = make(chan struct{})
	if s.outboxTicker == 0 {
		s.outboxTicker = defaultOutboxTicker
	}
	s.outboxWG.Add(1)
	go s.runOutboxWorker()
}

// Close stops the settlement-outbox worker (no-op if it was never started).
func (s *Service) Close() {
	if s.outboxStop != nil {
		close(s.outboxStop)
		s.outboxWG.Wait()
		s.outboxStop = nil
	}
}

func (s *Service) runOutboxWorker() {
	defer s.outboxWG.Done()
	ticker := time.NewTicker(s.outboxTicker)
	defer ticker.Stop()
	for {
		select {
		case <-s.outboxStop:
			return
		case <-ticker.C:
			s.drainSettlementOutbox(context.Background())
		}
	}
}

// drainSettlementOutbox processes one batch of due settlement jobs. Each job is
// handled under an advisory lock so concurrent workers/instances don't
// double-process the same order.
func (s *Service) drainSettlementOutbox(ctx context.Context) {
	orders, err := s.outbox.DueOrders(ctx, outboxBatch)
	if err != nil {
		slog.Error("settlement outbox: list due failed", "err", err)
		return
	}
	for _, orderID := range orders {
		s.settleOutboxOne(ctx, orderID)
	}
}

func (s *Service) settleOutboxOne(ctx context.Context, orderID string) {
	var settleErr error
	locked, lockErr := s.lock.WithLock(ctx, "settle:"+orderID, func(ctx context.Context) error {
		settleErr = s.settleOnce(ctx, orderID)
		if settleErr != nil {
			return s.outbox.MarkRetry(ctx, orderID, settleErr.Error(), settleBackoffBase, settleBackoffCap, maxSettleAttempts)
		}
		return s.outbox.MarkDone(ctx, orderID)
	})
	if lockErr != nil {
		slog.Error("settlement outbox: lock/update failed", "order_id", orderID, "err", lockErr)
		return
	}
	if !locked {
		return // another worker holds it; try again next tick
	}
	if settleErr != nil {
		slog.Warn("settlement outbox: retry scheduled", "order_id", orderID, "err", settleErr)
	}
}

// ListOutbox returns settlement outbox entries for ops monitoring. When status
// is empty all entries are returned; otherwise filters by status.
func (s *Service) ListOutbox(ctx context.Context, status string, limit, offset int) ([]OutboxEntry, error) {
	if s.outbox == nil {
		return nil, fmt.Errorf("outbox not enabled")
	}
	return s.outbox.ListOutbox(ctx, status, limit, offset)
}

// RetryOutbox resets a failed outbox entry so the background worker retries it
// on the next tick. Only callable on entries in 'failed' status.
func (s *Service) RetryOutbox(ctx context.Context, actorID, orderID string) error {
	if s.outbox == nil {
		return fmt.Errorf("outbox not enabled")
	}
	if err := s.outbox.RetryOutbox(ctx, orderID); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: actorID, Action: "settlement.outbox.retry",
		ResourceType: "order", ResourceID: orderID})
	return nil
}
