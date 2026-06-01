package payment

import (
	"context"
	"fmt"

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
}

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
	res, err := s.provider.CreatePayment(orderID, o.AmountCents)
	if err != nil {
		return PayInfo{}, fmt.Errorf("provider create payment: %w", err)
	}
	if err := s.repo.EnsurePayment(ctx, orderID, s.provider.Channel(), res.ChannelTxnID, o.AmountCents); err != nil {
		return PayInfo{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "payment.create", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"channel": s.provider.Channel(), "channel_txn_id": res.ChannelTxnID}})
	return PayInfo{OrderID: orderID, ChannelTxnID: res.ChannelTxnID, PayURL: res.PayURL, AmountCents: o.AmountCents, Channel: s.provider.Channel()}, nil
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

// Settle executes split-settlement for a confirmed order: seller share +
// platform commission move out of escrow at the provider, then the order
// becomes settled. Idempotent via the unique settlement row (the double-split
// guard, docs §6.6). NOTE: production should additionally take a distributed
// lock and use an outbox for the provider call + retries.
func (s *Service) Settle(ctx context.Context, orderID string) error {
	o, err := s.orders.GetSystem(ctx, orderID)
	if err != nil {
		return err
	}
	if o.Status != "confirmed" {
		return ErrNotConfirmed
	}
	created, err := s.repo.CreateSettlement(ctx, orderID, "settle:"+orderID, o.PlatformFeeCents, o.SellerAmountCents)
	if err != nil {
		return err
	}
	if !created {
		return nil // already settling/settled — idempotent
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
