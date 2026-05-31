package payment

import "errors"

// Payment statuses and escrow states (docs §5.3). Funds are escrowed at the
// licensed provider — NEVER in a platform account (docs §2.1, 资金二清 red line).
const (
	StatusCreated   = "created"
	StatusPaid      = "paid"
	StatusRefunded  = "refunded"
	StatusRefunding = "refunding"

	EscrowFrozen   = "frozen"
	EscrowReleased = "released"
	EscrowReverted = "reverted"
)

// Settlement (分账) statuses.
const (
	SettlePending  = "pending"
	SettleSuccess  = "success"
	SettleFailed   = "failed"
	SettleReverted = "reverted"
)

var (
	ErrOrderNotPayable  = errors.New("order is not in a payable state")
	ErrForbidden        = errors.New("not the buyer of this order")
	ErrInvalidSignature = errors.New("invalid callback signature")
	ErrNotConfirmed     = errors.New("order is not confirmed; cannot settle")
	ErrAlreadyHandled   = errors.New("callback already handled")
)

// PayInfo is returned to the buyer after creating a payment.
type PayInfo struct {
	OrderID      string `json:"order_id"`
	ChannelTxnID string `json:"channel_txn_id"`
	PayURL       string `json:"pay_url"` // QR / redirect URL from the provider
	AmountCents  int64  `json:"amount_cents"`
	Channel      string `json:"channel"`
}
