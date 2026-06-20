package apikey

import (
	"context"
	"time"
)

// Service is the API-key business layer for Oasis Verify.
type Service struct{ repo Repository }

// NewService wires a Service to its repository.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Issue mints a new key for an account and returns the ONE-TIME plaintext (the
// caller shows it to the user once and never stores it).
func (s *Service) Issue(ctx context.Context, accountID, name, tier string) (APIKey, string, error) {
	plaintext, prefix, hash, err := GenerateKey()
	if err != nil {
		return APIKey{}, "", err
	}
	k, err := s.repo.Create(ctx, APIKey{AccountID: accountID, Name: name, Prefix: prefix, Tier: tier}, hash)
	if err != nil {
		return APIKey{}, "", err
	}
	return k, plaintext, nil
}

// Authenticate validates a plaintext key and meters one scan against this month;
// returns ErrInvalidKey / ErrQuotaExceeded on failure.
func (s *Service) Authenticate(ctx context.Context, plaintext string) (APIKey, error) {
	return s.repo.AuthenticateAndMeter(ctx, HashKey(plaintext), CurrentMonth())
}

// List returns an account's keys (metadata only — never the plaintext).
func (s *Service) List(ctx context.Context, accountID string) ([]APIKey, error) {
	return s.repo.ListByAccount(ctx, accountID)
}

// Revoke disables a key the account owns.
func (s *Service) Revoke(ctx context.Context, accountID, id string) error {
	return s.repo.Revoke(ctx, accountID, id)
}

// CurrentMonth is the UTC 'YYYY-MM' bucket the usage counter resets on.
func CurrentMonth() string { return time.Now().UTC().Format("2006-01") }
