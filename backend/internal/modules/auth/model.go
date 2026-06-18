package auth

import (
	"errors"
	"time"
)

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
	TOTPEnabled bool   `json:"totp_enabled,omitempty"`
	TOTPSecret  string `json:"-"` // never serialised
	// TokensValidAfter is the session-invalidation epoch: a refresh token issued
	// before this instant is rejected (set on password reset). Nil = never
	// invalidated.
	TokensValidAfter *time.Time `json:"-"`
}

// Enroll2FAResult is returned when a user starts 2FA enrollment.
type Enroll2FAResult struct {
	OTPAuthURL    string   `json:"otpauth_url"`
	Secret        string   `json:"secret"`
	RecoveryCodes []string `json:"recovery_codes"` // only returned once
}

// LoginResult is the response from Login.
type LoginResult struct {
	User           *User   `json:"user,omitempty"`
	Tokens         *Tokens `json:"tokens,omitempty"`
	Need2FA        bool    `json:"need_2fa,omitempty"`
	ChallengeToken string  `json:"challenge_token,omitempty"`
}

// KYCRecord is a real-name verification submission (实名认证). The raw ID number
// is never stored — only id_no_hash (see hashIDNo); a production system should
// encrypt for retrievability, hashing is the MVP floor.
type KYCRecord struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	Type         string   `json:"type"` // personal | company
	RealName     string   `json:"real_name,omitempty"`
	CompanyName  string   `json:"company_name,omitempty"`
	MaterialURLs []string `json:"material_urls"`
	VerifyStatus string   `json:"verify_status"` // pending | verified | rejected
	CreatedAt    string   `json:"created_at,omitempty"`
}

// Agreement records a user accepting a legal document at a version. doc is a
// stable key ("terms" / "privacy" / "data_license"); version is the accepted
// document version. AgreedAt is populated by the store on read.
type Agreement struct {
	Doc      string `json:"doc"`
	Version  string `json:"version"`
	AgreedAt string `json:"agreed_at,omitempty"`
}

// Sentinel errors returned by the repository and service layers. Handlers map
// these onto httpx error codes; lower layers stay HTTP-agnostic.
var (
	ErrAccountExists         = errors.New("account already exists")
	ErrUserNotFound          = errors.New("user not found")
	ErrInvalidCredentials    = errors.New("invalid account or password")
	ErrUserFrozen            = errors.New("user is frozen")
	ErrInvalidToken          = errors.New("invalid or expired token")
	ErrValidation            = errors.New("validation failed")
	ErrKYCNotFound           = errors.New("no kyc record")
	ErrPayoutAccountNotFound = errors.New("no payout account")
	ErrAlready2FAEnabled     = errors.New("2fa already enabled")
	ErrNot2FAEnrolled        = errors.New("2fa not enrolled")
	ErrInvalid2FACode        = errors.New("invalid 2fa code or recovery code")
	ErrTokenInvalidOrExpired = errors.New("password reset token is invalid or expired")
	ErrPasswordTooWeak       = errors.New("password must be at least 8 characters")
)

const (
	accountTypePhone = "phone"
	accountTypeEmail = "email"

	statusActive = "active"
	statusFrozen = "frozen"

	kycTypePersonal = "personal"
	kycTypeCompany  = "company"

	kycNone     = "none"
	kycPending  = "pending"
	kycVerified = "verified"
	kycRejected = "rejected"

	roleBuyer  = "buyer"
	roleSeller = "seller"
	roleBoth   = "both"
	roleOps    = "ops"
	roleAdmin  = "admin"
)
