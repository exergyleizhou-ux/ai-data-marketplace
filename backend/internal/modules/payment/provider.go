package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// CreateResult is what a provider returns when a payment is initiated.
type CreateResult struct {
	ChannelTxnID string // provider-side transaction id
	PayURL       string // QR/redirect URL the buyer completes payment at
}

// CallbackResult is the verified outcome parsed from a provider webhook.
type CallbackResult struct {
	OrderID      string
	ChannelTxnID string
	Paid         bool
}

// PaymentProvider abstracts a licensed payment channel (WeChat/Alipay via an
// aggregator). CreatePayment initiates a charge with a split flag; VerifyCallback
// authenticates and parses the async notification.
//
// The real driver requires Spike-2 (分账可行性) + 法务 sign-off before any code
// touches production funds (docs §2.1/§8). MockProvider below is sandbox-only.
type PaymentProvider interface {
	Channel() string
	CreatePayment(orderID string, amountCents int64) (CreateResult, error)
	VerifyCallback(payload []byte, signature string) (CallbackResult, error)
}

// SplitProvider executes the split-settlement instruction at the licensed
// provider: it moves the seller's share + platform commission out of escrow.
// The platform never holds the funds — it only instructs (docs §2.1).
type SplitProvider interface {
	ExecuteSplit(orderID string, sellerAmountCents, platformFeeCents int64) (splitTxnID string, err error)
}

// MockProvider is a SANDBOX implementation for local/dev only. It does NOT move
// real money. Signatures are HMAC over a shared secret so the callback path
// (verify + idempotency) is exercised exactly as it will be in production.
type MockProvider struct{ Secret string }

func (m MockProvider) Channel() string { return "mock" }

func (m MockProvider) CreatePayment(orderID string, _ int64) (CreateResult, error) {
	txn := "mock-" + Sign(m.Secret, orderID)[:16]
	return CreateResult{
		ChannelTxnID: txn,
		PayURL:       "https://sandbox.example/pay?order=" + orderID + "&txn=" + txn,
	}, nil
}

// VerifyCallback expects payload "<orderID>:<channelTxnID>:<paid>" and signature
// = HMAC-SHA256(secret, payload).
func (m MockProvider) VerifyCallback(payload []byte, signature string) (CallbackResult, error) {
	if !hmac.Equal([]byte(signature), []byte(Sign(m.Secret, string(payload)))) {
		return CallbackResult{}, ErrInvalidSignature
	}
	parts := strings.SplitN(string(payload), ":", 3)
	if len(parts) != 3 {
		return CallbackResult{}, fmt.Errorf("malformed callback payload")
	}
	return CallbackResult{OrderID: parts[0], ChannelTxnID: parts[1], Paid: parts[2] == "true"}, nil
}

func (m MockProvider) ExecuteSplit(orderID string, _, _ int64) (string, error) {
	return "split-" + Sign(m.Secret, "split:"+orderID)[:16], nil
}

// Sign returns HMAC-SHA256(secret, msg) as hex — shared by the mock provider and
// tests to build valid callbacks.
func Sign(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
