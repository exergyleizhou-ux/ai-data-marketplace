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
	AutoConfirmAt     string `json:"auto_confirm_at,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

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
)

// platformFee splits an amount into (platformFee, sellerAmount) by basis points.
func platformFee(amount int64) (fee, seller int64) {
	fee = amount * platformFeeBps / 10000
	return fee, amount - fee
}
