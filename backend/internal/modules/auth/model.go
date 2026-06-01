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
	ErrAccountExists      = errors.New("account already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid account or password")
	ErrUserFrozen         = errors.New("user is frozen")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrValidation         = errors.New("validation failed")
	ErrKYCNotFound        = errors.New("no kyc record")
	// ErrPayoutAccountNotFound means the user has no active payout account for
	// the requested channel yet (the caller should create + persist one).
	ErrPayoutAccountNotFound = errors.New("no payout account")
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
