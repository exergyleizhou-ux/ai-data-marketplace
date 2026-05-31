package dataset

import "errors"

// Dataset is a sellable data product. Large bytes live in object storage; only
// metadata and version fingerprints live in Postgres.
type Dataset struct {
	ID                  string             `json:"id"`
	SellerID            string             `json:"seller_id"`
	Title               string             `json:"title"`
	Description         string             `json:"description"`
	DataType            string             `json:"data_type"`    // text | code | structured
	Domain              string             `json:"domain,omitempty"`
	LicenseType         string             `json:"license_type"` // commercial | research | train_only
	SuggestedPriceCents *int64             `json:"suggested_price_cents,omitempty"`
	FinalPriceCents     *int64             `json:"final_price_cents,omitempty"`
	Status              string             `json:"status"`
	TotalSizeBytes      int64              `json:"total_size_bytes"`
	SampleCount         int64              `json:"sample_count"`
	SourceDeclaration   *SourceDeclaration `json:"source_declaration,omitempty"`
	SourceSignedAt      string             `json:"source_signed_at,omitempty"`
	CurrentVersionID    string             `json:"current_version_id,omitempty"`
	CreatedAt           string             `json:"created_at,omitempty"`
	UpdatedAt           string             `json:"updated_at,omitempty"`
}

// SourceDeclaration is the seller's legally-binding statement about data
// provenance and licensing (docs §2.2). Signing it (source_signed_at) is a
// precondition for moving a dataset past draft.
type SourceDeclaration struct {
	Source           string `json:"source"`            // where the data came from
	CollectionMethod string `json:"collection_method"` // how it was collected
	ContainsPII      bool   `json:"contains_pii"`      // declares presence of personal info
	LicenseScope     string `json:"license_scope"`     // commercial | research | train_only
	Commitment       bool   `json:"commitment"`        // 承诺书 acknowledged
}

var (
	ErrNotFound      = errors.New("dataset not found")
	ErrValidation    = errors.New("validation failed")
	ErrForbidden     = errors.New("not the dataset owner")
	ErrNotVerified   = errors.New("seller must complete real-name verification")
	ErrNotEditable   = errors.New("dataset can only be edited in draft or rejected state")
	ErrNotSigned     = errors.New("source declaration must be signed first")
	ErrAlreadySigned = errors.New("source declaration already signed")
)

// Dataset statuses (docs §5.4 dataset lifecycle).
const (
	StatusDraft     = "draft"
	StatusUploading = "uploading"
	StatusChecking  = "checking"
	StatusReviewing = "reviewing"
	StatusPublished = "published"
	StatusRejected  = "rejected"
	StatusDelisted  = "delisted"
)

const (
	dataTypeText       = "text"
	dataTypeCode       = "code"
	dataTypeStructured = "structured"

	licenseCommercial = "commercial"
	licenseResearch   = "research"
	licenseTrainOnly  = "train_only"

	kycVerified = "verified"
)
