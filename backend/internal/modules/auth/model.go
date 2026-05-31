package auth

import "errors"

// User is the auth module's view of an account. Other modules receive identity
// through this type (or the middleware-injected id/role), never by reading the
// users table directly.
type User struct {
	ID          string `json:"id"`
	Account     string `json:"account"`
	AccountType string `json:"account_type"`
	Role        string `json:"role"`
	KYCStatus   string `json:"kyc_status"`
	Status      string `json:"status"`
}

// Sentinel errors returned by the repository and service layers. Handlers map
// these onto httpx error codes; lower layers stay HTTP-agnostic.
var (
	ErrAccountExists      = errors.New("account already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid account or password")
	ErrUserFrozen         = errors.New("user is frozen")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrValidation         = errors.New("validation failed")
)

const (
	accountTypePhone = "phone"
	accountTypeEmail = "email"

	statusActive = "active"
	statusFrozen = "frozen"
)
