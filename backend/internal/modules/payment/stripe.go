package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/account"
	"github.com/stripe/stripe-go/v79/paymentintent"
	"github.com/stripe/stripe-go/v79/transfer"
	"github.com/stripe/stripe-go/v79/webhook"
)

// StripeProvider is a REAL payment integration (test mode is free, no real
// money). It implements both PaymentProvider and SplitProvider using Stripe
// Connect's "separate charges and transfers" model, which matches our
// escrow-then-settle flow (docs §2.1, §4): the buyer's PaymentIntent lands in
// the platform balance (held); on buyer confirmation we Transfer the seller's
// share to their connected account and keep the platform commission.
//
// Connected-account ids are persisted in payout_accounts via PayoutAccountStore
// (H1) and additionally cached in-process for the lifetime of the run. Further
// production hardening: Connect onboarding/KYC and Balance-Transaction recon.
type StripeProvider struct {
	currency      string
	webhookSecret string
	store         PayoutAccountStore

	mu       sync.Mutex
	accounts map[string]string // sellerID -> connected account id (acct_...) — cache
}

// PayoutAccountStore persists the seller -> connected-account mapping so the id
// survives restarts. It is implemented by the auth module (which owns the
// payout_accounts table) and injected by the server, so payment never imports
// auth. PayoutAccountRef returns "" (no error) when the seller has none yet.
type PayoutAccountStore interface {
	PayoutAccountRef(ctx context.Context, sellerID, channel string) (string, error)
	SavePayoutAccount(ctx context.Context, sellerID, channel, accountRef string) error
}

// payoutChannel is the channel key under which Stripe connected accounts are
// stored in payout_accounts.
const payoutChannel = "stripe"

// NewStripeProvider sets the global Stripe key and returns the provider. store
// may be nil (the mapping then lives only in the in-process cache).
func NewStripeProvider(secretKey, webhookSecret, currency string, store PayoutAccountStore) *StripeProvider {
	stripe.Key = secretKey
	if currency == "" {
		currency = "usd"
	}
	return &StripeProvider{currency: currency, webhookSecret: webhookSecret, store: store, accounts: map[string]string{}}
}

func (p *StripeProvider) Channel() string { return "stripe" }

// CreatePayment creates a PaymentIntent on the platform account (funds are held
// in the platform balance until settlement). Returns the PI id + client secret.
func (p *StripeProvider) CreatePayment(orderID string, amountCents int64) (CreateResult, error) {
	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(amountCents),
		Currency:           stripe.String(p.currency),
		TransferGroup:      stripe.String(orderID),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
	}
	params.AddMetadata("order_id", orderID)
	pi, err := paymentintent.New(params)
	if err != nil {
		return CreateResult{}, fmt.Errorf("stripe payment intent: %w", err)
	}
	// PayURL carries the client secret; the frontend confirms with Stripe.js.
	return CreateResult{ChannelTxnID: pi.ID, PayURL: pi.ClientSecret}, nil
}

// VerifyCallback authenticates the Stripe-Signature header and parses the event.
// Only payment_intent.succeeded marks the order paid.
func (p *StripeProvider) VerifyCallback(payload []byte, signature string) (CallbackResult, error) {
	// IgnoreAPIVersionMismatch: the Stripe account may be on a newer API version
	// than this SDK pins; the signature is still valid, so don't reject on that.
	event, err := webhook.ConstructEventWithOptions(payload, signature, p.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return CallbackResult{}, ErrInvalidSignature
	}
	if event.Type != "payment_intent.succeeded" {
		return CallbackResult{Paid: false}, nil // ignore other events
	}
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return CallbackResult{}, fmt.Errorf("stripe event decode: %w", err)
	}
	return CallbackResult{OrderID: pi.Metadata["order_id"], ChannelTxnID: pi.ID, Paid: true}, nil
}

// ExecuteSplit transfers the seller's share from the platform balance to the
// seller's connected account (Connect transfer). Platform keeps the commission
// implicitly (it never transfers it out).
func (p *StripeProvider) ExecuteSplit(ctx context.Context, orderID, sellerRef string, sellerAmountCents, _ int64) (string, error) {
	acct, err := p.ensureAccount(ctx, sellerRef)
	if err != nil {
		return "", err
	}
	params := &stripe.TransferParams{
		Amount:        stripe.Int64(sellerAmountCents),
		Currency:      stripe.String(p.currency),
		Destination:   stripe.String(acct),
		TransferGroup: stripe.String(orderID),
	}
	params.Context = ctx
	tr, err := transfer.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe transfer: %w", err)
	}
	return tr.ID, nil
}

// ensureAccount returns the seller's connected account, resolving it in order:
// in-process cache -> persisted payout_accounts -> create a new fully-onboarded
// test Custom account (and persist it). Persistence (H1) means the acct_… id
// survives restarts instead of being recreated every boot.
func (p *StripeProvider) ensureAccount(ctx context.Context, sellerID string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.accounts[sellerID]; ok {
		return id, nil
	}
	// Persisted mapping (survives restarts).
	if p.store != nil {
		ref, err := p.store.PayoutAccountRef(ctx, sellerID, payoutChannel)
		if err != nil {
			return "", fmt.Errorf("load payout account: %w", err)
		}
		if ref != "" {
			p.accounts[sellerID] = ref
			return ref, nil
		}
	}
	params := &stripe.AccountParams{
		Type:         stripe.String("custom"),
		Country:      stripe.String("US"),
		BusinessType: stripe.String("individual"),
		Capabilities: &stripe.AccountCapabilitiesParams{
			Transfers: &stripe.AccountCapabilitiesTransfersParams{Requested: stripe.Bool(true)},
		},
		Individual: &stripe.PersonParams{
			FirstName: stripe.String("Test"),
			LastName:  stripe.String("Seller"),
			Email:     stripe.String("seller+" + sellerID[:8] + "@example.com"),
			DOB:       &stripe.PersonDOBParams{Day: stripe.Int64(1), Month: stripe.Int64(1), Year: stripe.Int64(1990)},
			IDNumber:  stripe.String("000000000"),
			SSNLast4:  stripe.String("0000"),
			Address: &stripe.AddressParams{
				Line1: stripe.String("123 Test St"), City: stripe.String("San Francisco"),
				State: stripe.String("CA"), PostalCode: stripe.String("94103"), Country: stripe.String("US"),
			},
			Phone: stripe.String("+15555550100"),
		},
		BusinessProfile: &stripe.AccountBusinessProfileParams{
			MCC: stripe.String("5734"), ProductDescription: stripe.String("AI training datasets seller"),
		},
		TOSAcceptance: &stripe.AccountTOSAcceptanceParams{
			Date: stripe.Int64(time.Now().Unix()), IP: stripe.String("127.0.0.1"),
		},
		ExternalAccount: &stripe.AccountExternalAccountParams{Token: stripe.String("btok_us_verified")},
	}
	acct, err := account.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe create connected account: %w", err)
	}
	if p.store != nil {
		if err := p.store.SavePayoutAccount(ctx, sellerID, payoutChannel, acct.ID); err != nil {
			return "", fmt.Errorf("persist payout account: %w", err)
		}
	}
	p.accounts[sellerID] = acct.ID
	return acct.ID, nil
}
