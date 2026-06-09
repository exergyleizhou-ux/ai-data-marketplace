package order

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// IdentityChecker reports a user's KYC status (implemented by auth).
type IdentityChecker interface {
	KYCStatus(ctx context.Context, userID string) (string, error)
}

// Purchasable is the subset of dataset info the order module needs.
type Purchasable struct {
	SellerID   string
	VersionID  string
	PriceCents int64
	Published  bool
}

// DatasetReader exposes purchase-relevant dataset info (implemented by dataset).
type DatasetReader interface {
	ForPurchase(ctx context.Context, datasetID string) (Purchasable, error)
}

// SettlementTrigger runs split-settlement once an order is confirmed. It is
// implemented by the payment module and injected by the server, so order does
// not import payment (avoids an import cycle).
type SettlementTrigger interface {
	Settle(ctx context.Context, orderID string) error
}

// RefundTrigger reverses a payment (refund + transfer reversal) when ops
// resolves a dispute for the buyer (H2). Like SettlementTrigger it is
// implemented by payment and injected late to avoid an import cycle.
type RefundTrigger interface {
	Refund(ctx context.Context, orderID string) error
}

// ComputeRevoker revokes a buyer's compute (C2D) entitlements tied to an order
// when that order is refunded. Implemented by the compute module and injected
// late so order does not import compute. Optional (may be nil).
type ComputeRevoker interface {
	RevokeEntitlementsForOrder(ctx context.Context, orderID string) (int, error)
}

// ComputeGranter grants the compute (C2D) entitlement for a PAID compute order.
// Implemented by the compute module, injected late. Must be idempotent. Optional.
type ComputeGranter interface {
	GrantForOrder(ctx context.Context, orderID, datasetID, buyerID string) error
}

// Notifier emits user-facing notifications for order lifecycle events.
// Implemented by the notification module; injected late. Optional.
type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

// Service holds order business logic and drives the status state machine.
type Service struct {
	repo     Repository
	identity IdentityChecker
	datasets DatasetReader
	audit    audit.Recorder
	settle   SettlementTrigger
	refund   RefundTrigger
	compute  ComputeRevoker
	granter  ComputeGranter
	notifier Notifier
}

func NewService(repo Repository, identity IdentityChecker, datasets DatasetReader, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	return &Service{repo: repo, identity: identity, datasets: datasets, audit: rec}
}

// SetSettlementTrigger wires the settlement hook after construction (the
// payment service needs this order service, so the dependency is set late).
func (s *Service) SetSettlementTrigger(t SettlementTrigger) { s.settle = t }

// SetRefundTrigger wires the refund hook (payment) after construction (H2).
func (s *Service) SetRefundTrigger(t RefundTrigger) { s.refund = t }

// SetComputeRevoker wires the compute-entitlement revoker (compute) so a refund
// also revokes the buyer's C2D credits for the order. Optional.
func (s *Service) SetComputeRevoker(r ComputeRevoker) { s.compute = r }

// SetComputeGranter wires the compute-entitlement granter (compute) so paying a
// compute order grants its entitlement. Optional.
func (s *Service) SetComputeGranter(g ComputeGranter) { s.granter = g }

// SetNotifier wires the notification emitter so order lifecycle events produce
// user-facing notifications. Optional (may be nil in tests).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// CreateCompute places a compute (C2D) order priced by the caller (the compute
// module passes the offer price). Same KYC / self-purchase / fee-split / state
// machine as a download; product_type='compute', no version.
func (s *Service) CreateCompute(ctx context.Context, buyerID, sellerID, datasetID string, amountCents int64) (Order, error) {
	if amountCents < 0 {
		return Order{}, fmt.Errorf("%w: amount", ErrValidation)
	}
	status, err := s.identity.KYCStatus(ctx, buyerID)
	if err != nil {
		return Order{}, err
	}
	if status != kycVerified {
		return Order{}, ErrNotVerified
	}
	if sellerID == buyerID {
		return Order{}, ErrSelfPurchase
	}
	fee, seller := platformFee(amountCents)
	o, err := s.repo.CreateCompute(ctx, Order{
		BuyerID: buyerID, SellerID: sellerID, DatasetID: datasetID,
		AmountCents: amountCents, PlatformFeeCents: fee, SellerAmountCents: seller,
	})
	if err != nil {
		return Order{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "order.create_compute", ResourceType: "order", ResourceID: o.ID,
		Detail: map[string]any{"dataset_id": datasetID, "amount_cents": amountCents}})
	return o, nil
}

// GetSystem returns an order without a party check — for internal callers
// (payment/settlement) acting as the system, not an end user.
func (s *Service) GetSystem(ctx context.Context, id string) (Order, error) {
	return s.repo.GetByID(ctx, id)
}

const kycVerified = "verified"

// Create places an order. The buyer must be real-name verified; the dataset
// must be published; self-purchase and duplicate active orders are rejected.
func (s *Service) Create(ctx context.Context, buyerID, datasetID, licenseType string) (Order, error) {
	switch licenseType {
	case "commercial", "research", "train_only":
	default:
		return Order{}, fmt.Errorf("%w: invalid license_type", ErrValidation)
	}
	status, err := s.identity.KYCStatus(ctx, buyerID)
	if err != nil {
		return Order{}, err
	}
	if status != kycVerified {
		return Order{}, ErrNotVerified
	}
	ds, err := s.datasets.ForPurchase(ctx, datasetID)
	if err != nil {
		return Order{}, err
	}
	if !ds.Published || ds.VersionID == "" {
		return Order{}, ErrNotPurchasable
	}
	if ds.SellerID == buyerID {
		return Order{}, ErrSelfPurchase
	}
	fee, seller := platformFee(ds.PriceCents)
	o, err := s.repo.Create(ctx, Order{
		BuyerID: buyerID, SellerID: ds.SellerID, DatasetID: datasetID, VersionID: ds.VersionID,
		LicenseType: licenseType, AmountCents: ds.PriceCents, PlatformFeeCents: fee, SellerAmountCents: seller,
	})
	if err != nil {
		return Order{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "order.create", ResourceType: "order", ResourceID: o.ID,
		Detail: map[string]any{"dataset_id": datasetID, "amount_cents": ds.PriceCents}})
	return o, nil
}

// Get returns an order; only its buyer or seller may view it.
func (s *Service) Get(ctx context.Context, userID, id string) (Order, error) {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if o.BuyerID != userID && o.SellerID != userID {
		return Order{}, ErrForbidden
	}
	return o, nil
}

func (s *Service) ListMine(ctx context.Context, buyerID string, limit, offset int) ([]Order, error) {
	return s.repo.ListByBuyer(ctx, buyerID, clampLimit(limit), max0(offset))
}
func (s *Service) ListSales(ctx context.Context, sellerID string, limit, offset int) ([]Order, error) {
	return s.repo.ListBySeller(ctx, sellerID, clampLimit(limit), max0(offset))
}

// --- state-machine transitions ---

func (s *Service) transition(ctx context.Context, actorID, id, from, to string, setAutoConfirm bool) (Order, error) {
	o, err := s.repo.Transition(ctx, id, from, to, setAutoConfirm)
	if err != nil {
		return Order{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: actorID, Action: "order." + to, ResourceType: "order", ResourceID: id})
	return o, nil
}

// MarkPaid: created -> paid (called by the payment module on a verified callback).
// For a COMPUTE order, payment is the moment of delivery: grant the compute
// entitlement (idempotent) and auto-advance paid -> delivered (arming the
// auto-confirm + settlement that pays the seller). A grant/deliver hiccup leaves
// the order 'paid' for a retry rather than failing the payment callback.
func (s *Service) MarkPaid(ctx context.Context, id string) (Order, error) {
	paid, err := s.transition(ctx, "", id, StatusCreated, StatusPaid, false)
	if err != nil {
		return Order{}, err
	}
	if paid.ProductType != ProductCompute {
		if s.notifier != nil {
			_ = s.notifier.NotifyUser(ctx, paid.BuyerID, "order_paid",
				"订单已支付", "您的订单 #"+trunc8(paid.ID)+" 已支付成功，等待交付。",
				"order", paid.ID)
		}
		return paid, nil
	}
	if s.granter != nil {
		if err := s.granter.GrantForOrder(ctx, paid.ID, paid.DatasetID, paid.BuyerID); err != nil {
			slog.Error("compute entitlement grant on paid failed (order left paid, retriable)", "order_id", id, "err", err)
			return paid, nil
		}
	}
	delivered, err := s.transition(ctx, "", id, StatusPaid, StatusDelivered, true)
	if err != nil {
		slog.Error("compute order auto-deliver failed (entitlement granted, order left paid)", "order_id", id, "err", err)
		return paid, nil
	}
	return delivered, nil
}

// MarkDelivered: paid -> delivered, arming the 7-day auto-confirm (called by delivery).
func (s *Service) MarkDelivered(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "", id, StatusPaid, StatusDelivered, true)
}

// ConfirmDelivery: delivered -> confirmed (buyer action or auto-confirm).
func (s *Service) ConfirmDelivery(ctx context.Context, buyerID, id string) (Order, error) {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if o.BuyerID != buyerID {
		return Order{}, ErrForbidden
	}
	if _, err := s.transition(ctx, buyerID, id, StatusDelivered, StatusConfirmed, false); err != nil {
		return Order{}, err
	}
	// Trigger split-settlement. It is idempotent and retriable, so a failure
	// here leaves the order confirmed (settlement can be re-run) rather than
	// blocking the buyer's confirmation — but log it (don't swallow silently).
	if s.settle != nil {
		if err := s.settle.Settle(ctx, id); err != nil {
			slog.Error("settlement failed (order left confirmed, retriable)", "order_id", id, "err", err)
		}
	}
	return s.repo.GetByID(ctx, id)
}

// MarkSettled: confirmed -> settled (called by the settlement module). It is
// idempotent: if the order is already settled (a retried settlement re-running
// after the first one flipped it) it returns the order without error, so the
// outbox retry path (H3) doesn't fail on an already-settled order.
func (s *Service) MarkSettled(ctx context.Context, id string) (Order, error) {
	if o, err := s.repo.GetByID(ctx, id); err == nil && o.Status == StatusSettled {
		return o, nil
	}
	settled, err := s.transition(ctx, "", id, StatusConfirmed, StatusSettled, false)
	if err == nil && s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, settled.SellerID, "order_settled",
			"订单已结算", "订单 #"+trunc8(settled.ID)+" 已结算，金额 ¥"+centsStr(settled.SellerAmountCents)+" 已分账至您的账户。",
			"order", settled.ID)
	}
	return settled, err
}

// Cancel: created -> cancelled (payment timeout).
func (s *Service) Cancel(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "", id, StatusCreated, StatusCancelled, false)
}

// Dispute moves an active order to disputed and records the dispute.
func (s *Service) Dispute(ctx context.Context, userID, id, reason string) (Order, error) {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if o.BuyerID != userID && o.SellerID != userID {
		return Order{}, ErrForbidden
	}
	switch o.Status {
	case StatusPaid, StatusDelivered, StatusConfirmed:
	default:
		return Order{}, ErrBadTransition
	}
	if err := s.repo.CreateDispute(ctx, id, userID, reason); err != nil {
		return Order{}, err
	}
	result, err := s.transition(ctx, userID, id, o.Status, StatusDisputed, false)
	if err == nil && s.notifier != nil {
		other := result.SellerID
		if userID == result.SellerID {
			other = result.BuyerID
		}
		_ = s.notifier.NotifyUser(ctx, other, "order_disputed",
			"订单纠纷", "订单 #"+trunc8(result.ID)+" 已被对方提起纠纷。",
			"order", result.ID)
	}
	return result, err
}

// Earnings returns the seller's money summary.
func (s *Service) Earnings(ctx context.Context, sellerID string) (Earnings, error) {
	return s.repo.SellerEarnings(ctx, sellerID)
}

// AdminTransactions lists all orders (ops view of流水 + commission).
func (s *Service) AdminTransactions(ctx context.Context, limit, offset int) ([]Order, error) {
	return s.repo.AdminList(ctx, clampLimit(limit), max0(offset))
}

// AdminReconciliation returns aggregate financial stats for the ops dashboard.
func (s *Service) AdminReconciliation(ctx context.Context) (Reconciliation, error) {
	return s.repo.AdminReconciliation(ctx)
}

// AdminReconciliationTimeseries returns daily aggregates for ops.
func (s *Service) AdminReconciliationTimeseries(ctx context.Context, days int) ([]ReconciliationPoint, error) {
	return s.repo.AdminReconciliationTimeseries(ctx, days)
}

// SellerEarningsTimeseries returns the seller's daily earnings.
func (s *Service) SellerEarningsTimeseries(ctx context.Context, sellerID string, days int) ([]EarningsPoint, error) {
	return s.repo.SellerEarningsTimeseries(ctx, sellerID, days)
}

// SellerEarningsByDataset returns per-dataset earnings for a seller.
func (s *Service) SellerEarningsByDataset(ctx context.Context, sellerID string) ([]EarningsByDataset, error) {
	return s.repo.SellerEarningsByDataset(ctx, sellerID)
}

// CreateReview lets the buyer rate a settled order (one review per order).
func (s *Service) CreateReview(ctx context.Context, buyerID, orderID string, score int, comment string, issueFlag bool) (Review, error) {
	if score < 1 || score > 5 {
		return Review{}, fmt.Errorf("%w: score must be 1..5", ErrValidation)
	}
	o, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return Review{}, err
	}
	if o.BuyerID != buyerID {
		return Review{}, ErrForbidden
	}
	if o.Status != StatusSettled {
		return Review{}, ErrNotSettled
	}
	rv, err := s.repo.CreateReview(ctx, Review{
		OrderID: orderID, DatasetID: o.DatasetID, BuyerID: buyerID, Score: score, Comment: comment, IssueFlag: issueFlag,
	})
	if err != nil {
		return Review{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "review.create", ResourceType: "dataset", ResourceID: o.DatasetID,
		Detail: map[string]any{"order_id": orderID, "score": score, "issue": issueFlag}})
	return rv, nil
}

// ListReviews returns a dataset's reviews.
func (s *Service) ListReviews(ctx context.Context, datasetID string, limit, offset int) ([]Review, error) {
	return s.repo.ListReviewsByDataset(ctx, datasetID, clampLimit(limit), max0(offset))
}

// ResolveDispute is the ops decision on a disputed order: refund (-> refunded)
// or release (-> confirmed, then auto-settle).
//
// For a refund the real money movement (provider refund + 分账回退 transfer
// reversal) runs FIRST via the refund trigger; only if it succeeds do we flip
// the dispute/order state, so a provider failure leaves the dispute open for a
// retry rather than marking refunded with funds still moved.
func (s *Service) ResolveDispute(ctx context.Context, opsID, orderID string, refund bool, note string) (Order, error) {
	o, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if o.Status != StatusDisputed {
		return Order{}, ErrNotDisputed
	}
	if refund {
		if s.refund != nil {
			if err := s.refund.Refund(ctx, orderID); err != nil {
				return Order{}, fmt.Errorf("refund: %w", err)
			}
		}
		if err := s.repo.ResolveDispute(ctx, orderID, "resolved_refund", note, opsID); err != nil {
			return Order{}, err
		}
		refunded, err := s.transition(ctx, opsID, orderID, StatusDisputed, StatusRefunded, false)
		if err != nil {
			return Order{}, err
		}
		// Revoke any compute (C2D) entitlements bought via this order. Best-effort:
		// the refund already succeeded, so a revoke hiccup is logged, not fatal.
		if s.compute != nil {
			if _, rerr := s.compute.RevokeEntitlementsForOrder(ctx, orderID); rerr != nil {
				slog.Error("compute entitlement revoke after refund failed (retriable)", "order_id", orderID, "err", rerr)
			}
		}
		return refunded, nil
	}
	if err := s.repo.ResolveDispute(ctx, orderID, "resolved_release", note, opsID); err != nil {
		return Order{}, err
	}
	released, err := s.transition(ctx, opsID, orderID, StatusDisputed, StatusConfirmed, false)
	if err != nil {
		return Order{}, err
	}
	if s.settle != nil {
		_ = s.settle.Settle(ctx, orderID)
	}
	return s.repo.GetByID(ctx, released.ID)
}

func clampLimit(l int) int {
	if l <= 0 || l > 100 {
		return 20
	}
	return l
}
func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func trunc8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func centsStr(c int64) string {
	return fmt.Sprintf("%.2f", float64(c)/100)
}
