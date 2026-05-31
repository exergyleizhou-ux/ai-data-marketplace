package order

import (
	"context"
	"fmt"

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

// Service holds order business logic and drives the status state machine.
type Service struct {
	repo     Repository
	identity IdentityChecker
	datasets DatasetReader
	audit    audit.Recorder
}

func NewService(repo Repository, identity IdentityChecker, datasets DatasetReader, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	return &Service{repo: repo, identity: identity, datasets: datasets, audit: rec}
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
func (s *Service) MarkPaid(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "system", id, StatusCreated, StatusPaid, false)
}

// MarkDelivered: paid -> delivered, arming the 7-day auto-confirm (called by delivery).
func (s *Service) MarkDelivered(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "system", id, StatusPaid, StatusDelivered, true)
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
	return s.transition(ctx, buyerID, id, StatusDelivered, StatusConfirmed, false)
}

// MarkSettled: confirmed -> settled (called by the settlement module).
func (s *Service) MarkSettled(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "system", id, StatusConfirmed, StatusSettled, false)
}

// Cancel: created -> cancelled (payment timeout).
func (s *Service) Cancel(ctx context.Context, id string) (Order, error) {
	return s.transition(ctx, "system", id, StatusCreated, StatusCancelled, false)
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
	return s.transition(ctx, userID, id, o.Status, StatusDisputed, false)
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
