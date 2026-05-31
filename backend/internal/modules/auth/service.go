package auth

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Service holds the auth business logic. It is HTTP-agnostic: it returns the
// sentinel errors from model.go, which handlers translate to httpx codes.
type Service struct {
	repo      Repository
	tokens    *TokenManager
	verifier  KYCVerifier
	piiSecret string
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithKYC sets the real-name verification backend and the secret used to hash
// ID numbers before storage.
func WithKYC(verifier KYCVerifier, piiSecret string) Option {
	return func(s *Service) {
		s.verifier = verifier
		s.piiSecret = piiSecret
	}
}

func NewService(repo Repository, tokens *TokenManager, opts ...Option) *Service {
	s := &Service{repo: repo, tokens: tokens, verifier: ManualVerifier{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// AuthResult bundles the user and freshly issued tokens.
type AuthResult struct {
	User   User   `json:"user"`
	Tokens Tokens `json:"tokens"`
}

const minPasswordLen = 8

// Register creates an account and returns an initial token pair.
func (s *Service) Register(ctx context.Context, account, accountType, password string) (AuthResult, error) {
	account = strings.TrimSpace(account)
	if err := validateCredentials(account, accountType, password); err != nil {
		return AuthResult{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.repo.CreateUser(ctx, account, accountType, string(hash))
	if err != nil {
		return AuthResult{}, err // ErrAccountExists or wrapped internal
	}
	return s.issue(user)
}

// Login verifies credentials and returns a token pair.
func (s *Service) Login(ctx context.Context, account, password string) (AuthResult, error) {
	account = strings.TrimSpace(account)
	user, hash, err := s.repo.GetUserByAccount(ctx, account)
	if err != nil {
		// Do not distinguish "no such account" from "wrong password".
		return AuthResult{}, ErrInvalidCredentials
	}
	if user.Status == statusFrozen {
		return AuthResult{}, ErrUserFrozen
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	return s.issue(user)
}

// Refresh exchanges a valid refresh token for a new token pair, re-checking
// that the user still exists and is active.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	claims, err := s.tokens.Parse(refreshToken, tokenTypeRefresh)
	if err != nil {
		return AuthResult{}, ErrInvalidToken
	}
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return AuthResult{}, ErrInvalidToken
	}
	if user.Status == statusFrozen {
		return AuthResult{}, ErrUserFrozen
	}
	return s.issue(user)
}

// Me returns the current user's profile by id (used by GET /users/me).
func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

// KYCStatus returns the user's real-name verification status. It satisfies the
// identity-check interface other modules (e.g. dataset) depend on, so they can
// gate seller actions without importing auth internals or the users table.
func (s *Service) KYCStatus(ctx context.Context, userID string) (string, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return u.KYCStatus, nil
}

func (s *Service) issue(user User) (AuthResult, error) {
	tokens, err := s.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return AuthResult{}, fmt.Errorf("issue tokens: %w", err)
	}
	return AuthResult{User: user, Tokens: tokens}, nil
}

func validateCredentials(account, accountType, password string) error {
	if account == "" {
		return fmt.Errorf("%w: account is required", ErrValidation)
	}
	if accountType != accountTypePhone && accountType != accountTypeEmail {
		return fmt.Errorf("%w: account_type must be phone or email", ErrValidation)
	}
	if len(password) < minPasswordLen {
		return fmt.Errorf("%w: password must be at least %d characters", ErrValidation, minPasswordLen)
	}
	return nil
}
