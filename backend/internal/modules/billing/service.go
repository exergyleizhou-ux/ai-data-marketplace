package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/webhook"
)

var (
	// ErrDisabled means no Stripe keys are configured (paid plans are off; the
	// free tier still works).
	ErrDisabled = errors.New("billing is not configured")
	// ErrInvalidSignature means the webhook signature did not verify.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrUnknownPrice means the requested price is not in the tier map.
	ErrUnknownPrice = errors.New("unknown price")
)

// TierSetter applies a plan tier to an account. apikey.Service satisfies it.
type TierSetter interface {
	SetTier(ctx context.Context, accountID, tier string) (int, error)
}

// Service maps Stripe Checkout + subscription webhooks to Oasis Verify tiers.
type Service struct {
	secretKey     string
	webhookSecret string
	priceMap      map[string]string
	appBaseURL    string
	tiers         TierSetter
}

// NewService builds a billing Service. With an empty secretKey it is disabled.
func NewService(secretKey, webhookSecret, priceMapRaw, appBaseURL string, tiers TierSetter) *Service {
	return &Service{
		secretKey: secretKey, webhookSecret: webhookSecret,
		priceMap: parsePriceMap(priceMapRaw), appBaseURL: appBaseURL, tiers: tiers,
	}
}

// Enabled reports whether paid plans are configured (both keys present).
func (s *Service) Enabled() bool { return s.secretKey != "" && s.webhookSecret != "" }

// CheckoutURL creates a Stripe Checkout subscription session and returns its URL.
// account_id is stamped on the subscription metadata so the webhook can map the
// eventual subscription back to the account.
func (s *Service) CheckoutURL(ctx context.Context, accountID, priceID string) (string, error) {
	if s.secretKey == "" {
		return "", ErrDisabled
	}
	if _, ok := s.priceMap[priceID]; !ok {
		return "", ErrUnknownPrice
	}
	stripe.Key = s.secretKey
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems:  []*stripe.CheckoutSessionLineItemParams{{Price: stripe.String(priceID), Quantity: stripe.Int64(1)}},
		SuccessURL: stripe.String(s.appBaseURL + "/verify-api?upgraded=1"),
		CancelURL:  stripe.String(s.appBaseURL + "/verify-api"),
		Metadata:   map[string]string{"account_id": accountID},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{"account_id": accountID},
		},
	}
	params.Context = ctx
	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	return sess.URL, nil
}

// HandleWebhook verifies a Stripe webhook signature and applies any plan-tier
// change. A disabled service no-ops; a bad signature errors.
func (s *Service) HandleWebhook(payload []byte, sigHeader string) error {
	if s.webhookSecret == "" {
		return ErrDisabled
	}
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, s.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return ErrInvalidSignature
	}
	return s.applyVerifiedEvent(event)
}

// applyVerifiedEvent maps a verified subscription event to a tier change. Split
// out from signature verification so the dispatch is unit-testable.
func (s *Service) applyVerifiedEvent(event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return nil // not a subscription event we handle
	}
	priceID := ""
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		priceID = sub.Items.Data[0].Price.ID
	}
	tier, apply := resolveTierChange(string(event.Type), priceID, s.priceMap)
	accountID := sub.Metadata["account_id"]
	if !apply || accountID == "" {
		return nil
	}
	if _, err := s.tiers.SetTier(context.Background(), accountID, tier); err != nil {
		slog.Error("billing: set tier", "account", accountID, "tier", tier, "err", err)
		return err
	}
	slog.Info("billing: tier applied", "account", accountID, "tier", tier, "event", event.Type)
	return nil
}
