package order

import "errors"

// Order is a purchase of a dataset version. Money is integer cents.
type Order struct {
	ID                string `json:"id"`
	BuyerID           string `json:"buyer_id"`
	SellerID          string `json:"seller_id"`
	DatasetID         string `json:"dataset_id"`
	VersionID         string `json:"version_id"`
	LicenseType       string `json:"license_type"`
	AmountCents       int64  `json:"amount_cents"`
	PlatformFeeCents  int64  `json:"platform_fee_cents"`
	SellerAmountCents int64  `json:"seller_amount_cents"`
	Status            string `json:"status"`
	ProductType       string `json:"product_type"` // download | compute
	AutoConfirmAt     string `json:"auto_confirm_at,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// Product types: a download order delivers dataset bytes; a compute order grants
// a compute (C2D) entitlement on payment (design §10).
const (
	ProductDownload = "download"
	ProductCompute  = "compute"
)

// Order statuses and the state machine (docs §5.4).
const (
	StatusCreated   = "created"
	StatusPaid      = "paid"
	StatusDelivered = "delivered"
	StatusConfirmed = "confirmed"
	StatusSettled   = "settled"
	StatusDisputed  = "disputed"
	StatusRefunded  = "refunded"
	StatusCancelled = "cancelled"
)

// platformFeeBps is the platform commission in basis points (10% = 1000 bps).
const platformFeeBps = 1000

var (
	ErrNotFound       = errors.New("order not found")
	ErrValidation     = errors.New("validation failed")
	ErrForbidden      = errors.New("not a party to this order")
	ErrNotVerified    = errors.New("buyer must complete real-name verification")
	ErrNotPurchasable = errors.New("dataset is not available for purchase")
	ErrSelfPurchase   = errors.New("cannot buy your own dataset")
	ErrDuplicateOrder = errors.New("an active order for this dataset already exists")
	ErrBadTransition  = errors.New("illegal order status transition")
	ErrReviewExists   = errors.New("order already reviewed")
	ErrNotSettled     = errors.New("can only review a settled order")
	ErrNotDisputed    = errors.New("order is not in dispute")
)

// Review is a buyer's rating of a completed purchase.
type Review struct {
	ID        string `json:"id"`
	OrderID   string `json:"order_id"`
	DatasetID string `json:"dataset_id"`
	BuyerID   string `json:"buyer_id"`
	Score     int    `json:"score"`
	Comment   string `json:"comment,omitempty"`
	IssueFlag bool   `json:"issue_flag"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Reconciliation summarizes the platform's financial state for ops.
type Reconciliation struct {
	TotalGMV          int64 `json:"total_gmv"`
	SettledGMV        int64 `json:"settled_gmv"`
	PlatformFees      int64 `json:"platform_fees"`
	TotalOrders       int64 `json:"total_orders"`
	SettledOrders     int64 `json:"settled_orders"`
	PendingOrders     int64 `json:"pending_orders"`
	DisputedOrders    int64 `json:"disputed_orders"`
	RefundedOrders    int64 `json:"refunded_orders"`
	RefundedAmount    int64 `json:"refunded_amount"`
	FailedSettlements int64 `json:"failed_settlements"`
}

// Earnings summarizes a seller's money across orders (integer cents).
type Earnings struct {
	SettledCents      int64 `json:"settled_cents"`      // realized (settled orders)
	PendingCents      int64 `json:"pending_cents"`      // paid/delivered/confirmed, not yet settled
	WithdrawableCents int64 `json:"withdrawable_cents"` // == settled in MVP (funds at provider)
	SettledOrders     int   `json:"settled_orders"`
	PendingOrders     int   `json:"pending_orders"`
}

// platformFee splits an amount into (platformFee, sellerAmount) by basis points.
func platformFee(amount int64) (fee, seller int64) {
	fee = amount * platformFeeBps / 10000
	return fee, amount - fee
}
