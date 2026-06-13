package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Service holds the auth business logic. It is HTTP-agnostic: it returns the
// sentinel errors from model.go, which handlers translate to httpx codes.
type Service struct {
	repo       Repository
	tokens     *TokenManager
	verifier   KYCVerifier
	piiSecret  string
	denylist   Denylist
	notifier   Notifier
	appBaseURL string
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

// WithDenylist wires a refresh-token revocation backend. Without it the service
// defaults to a no-op denylist (stateless tokens, no revocation).
func WithDenylist(dl Denylist) Option {
	return func(s *Service) { s.denylist = dl }
}

// Notifier is the notification interface used for password reset emails.
type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

// SetNotifier wires the notification emitter.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// TokenManager returns the underlying token issuer/verifier (used by router and tests).
func (s *Service) TokenManager() *TokenManager { return s.tokens }

// SetAppBaseURL sets the base URL for email links.
func (s *Service) SetAppBaseURL(url string) { s.appBaseURL = url }

func NewService(repo Repository, tokens *TokenManager, opts ...Option) *Service {
	s := &Service{repo: repo, tokens: tokens, verifier: ManualVerifier{}, denylist: noopDenylist{}}
	for _, o := range opts {
		o(s)
	}
	if s.denylist == nil {
		s.denylist = noopDenylist{}
	}
	return s
}

// AuthResult bundles the user and freshly issued tokens.
type AuthResult struct {
	User   User   `json:"user"`
	Tokens Tokens `json:"tokens"`
}

const minPasswordLen = 8

// Register creates an account and returns an initial token pair. Any agreements
// passed (e.g. terms/privacy accepted at sign-up) are recorded as consent so a
// later policy update can require re-consent against an auditable trail.
func (s *Service) Register(ctx context.Context, account, accountType, password string, agreements ...Agreement) (AuthResult, error) {
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
	if len(agreements) > 0 {
		if err := s.repo.RecordAgreements(ctx, user.ID, agreements); err != nil {
			return AuthResult{}, fmt.Errorf("record agreements: %w", err)
		}
	}
	return s.issue(user)
}

// RecordAgreements appends consent records for a user (e.g. re-consent after a
// policy version bump).
func (s *Service) RecordAgreements(ctx context.Context, userID string, ags []Agreement) error {
	return s.repo.RecordAgreements(ctx, userID, ags)
}

// ListAgreements returns a user's consent history, most recent first.
func (s *Service) ListAgreements(ctx context.Context, userID string) ([]Agreement, error) {
	return s.repo.ListAgreements(ctx, userID)
}

const (
	// maxLoginFailures consecutive bad passwords lock the account for
	// loginLockoutWindow. The lock answers with the same ErrInvalidCredentials
	// (no enumeration); the counter resets on success, lock expiry, or a
	// completed password reset.
	maxLoginFailures   = 5
	loginLockoutWindow = 15 * time.Minute
)

// Login verifies credentials and returns a token pair, or a 2FA challenge if
// TOTP is enabled for this account.
func (s *Service) Login(ctx context.Context, account, password string) (LoginResult, error) {
	account = strings.TrimSpace(account)
	user, hash, err := s.repo.GetUserByAccount(ctx, account)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if user.Status == statusFrozen {
		return LoginResult{}, ErrUserFrozen
	}
	// Account lockout: same generic error as a wrong password so a locked
	// state never confirms account existence (anti-enumeration). Repo errors
	// fail open — bcrypt below still gates — mirroring the rate limiter.
	if _, locked, err := s.repo.LoginLockedUntil(ctx, user.ID); err == nil && locked {
		slog.Warn("login attempt on locked account", "user_id", user.ID)
		return LoginResult{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		if n, until, err := s.repo.RecordLoginFailure(ctx, user.ID, maxLoginFailures, loginLockoutWindow); err == nil && !until.IsZero() {
			slog.Warn("account locked after repeated login failures",
				"user_id", user.ID, "failures", n, "locked_until", until)
		}
		return LoginResult{}, ErrInvalidCredentials
	}
	_ = s.repo.ClearLoginFailures(ctx, user.ID)
	// GetByAccount doesn't return totp_enabled. Reload.
	u2, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		u2 = user
	}
	if u2.TOTPEnabled {
		challenge, err := s.tokens.Issue2FAChallenge(user.ID)
		if err != nil {
			return LoginResult{}, fmt.Errorf("issue challenge: %w", err)
		}
		return LoginResult{Need2FA: true, ChallengeToken: challenge, User: &u2}, nil
	}
	tokens, err := s.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue tokens: %w", err)
	}
	return LoginResult{User: &u2, Tokens: &tokens}, nil
}

// Refresh exchanges a valid refresh token for a new token pair, re-checking
// that the user still exists and is active.
//
// Refresh tokens are single-use: a successful refresh revokes the presented
// token (rotation) and issues a fresh pair. Presenting an already-rotated or
// logged-out token is rejected (reuse detection). On a transient denylist
// (Redis) error the check fails open and is logged, consistent with the rate
// limiter — a structurally valid, signed, unexpired token is still required.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	claims, err := s.tokens.Parse(refreshToken, tokenTypeRefresh)
	if err != nil {
		return AuthResult{}, ErrInvalidToken
	}
	if claims.ID != "" {
		if revoked, err := s.denylist.IsRevoked(ctx, claims.ID); err != nil {
			slog.Error("denylist check failed; denying refresh", "err", err)
			return AuthResult{}, ErrInvalidToken
		} else if revoked {
			return AuthResult{}, ErrInvalidToken
		}
	}
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return AuthResult{}, ErrInvalidToken
	}
	if user.Status == statusFrozen {
		return AuthResult{}, ErrUserFrozen
	}
	// Rotation: the presented refresh token is now spent.
	if claims.ID != "" && claims.ExpiresAt != nil {
		if err := s.denylist.Revoke(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			slog.Warn("failed to revoke rotated refresh token", "err", err)
		}
	}
	return s.issue(user)
}

// Logout revokes a refresh token so the session can no longer be renewed; the
// matching access token expires on its own short TTL. It is idempotent: an
// invalid or already-expired token is treated as success (nothing to revoke).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokens.Parse(refreshToken, tokenTypeRefresh)
	if err != nil || claims.ID == "" || claims.ExpiresAt == nil {
		return nil
	}
	return s.denylist.Revoke(ctx, claims.ID, time.Until(claims.ExpiresAt.Time))
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

// PayoutAccountRef returns the user's persisted channel-side payout account ref
// (e.g. a Stripe Connect acct_… id), or ErrPayoutAccountNotFound. The payment
// module reads this through an adapter so it never touches the payout table.
func (s *Service) PayoutAccountRef(ctx context.Context, userID, channel string) (string, error) {
	return s.repo.GetPayoutAccountRef(ctx, userID, channel)
}

// SavePayoutAccount persists (upserts) a user's channel-side payout account ref.
func (s *Service) SavePayoutAccount(ctx context.Context, userID, channel, accountRef string) error {
	return s.repo.SavePayoutAccount(ctx, userID, channel, accountRef)
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
