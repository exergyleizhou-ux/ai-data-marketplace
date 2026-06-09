package withdrawal

import "errors"

type Request struct {
	ID           string `json:"id"`
	SellerID     string `json:"seller_id"`
	AmountCents  int64  `json:"amount_cents"`
	Channel      string `json:"channel"`
	AccountLabel string `json:"account_label"`
	Status       string `json:"status"`
	OpsNote      string `json:"ops_note,omitempty"`
	RequestedAt  string `json:"requested_at"`
	ProcessedAt  string `json:"processed_at,omitempty"`
	ProcessedBy  string `json:"processed_by,omitempty"`
}

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusCompleted = "completed"
	StatusRejected  = "rejected"
)

var (
	ErrInsufficientBalance = errors.New("insufficient settled balance")
	ErrBadTransition       = errors.New("illegal status transition")
	ErrNotFound            = errors.New("withdrawal not found")
	ErrForbidden           = errors.New("not your withdrawal")
	ErrAmountInvalid       = errors.New("amount must be > 0 and <= 1,000,000 yuan")
	ErrChannelInvalid      = errors.New("channel must be wechat|alipay|bank")
	ErrReasonRequired      = errors.New("reject reason is required")
)
