package dataset

import (
	"context"
	"fmt"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// IdentityChecker lets the dataset module ask the identity module (auth)
// whether a user is real-name verified, without importing auth or touching the
// users table (modular-monolith boundary).
type IdentityChecker interface {
	KYCStatus(ctx context.Context, userID string) (string, error)
}

// Service holds dataset business logic.
type Service struct {
	repo     Repository
	identity IdentityChecker
	audit    audit.Recorder
}

func NewService(repo Repository, identity IdentityChecker, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	return &Service{repo: repo, identity: identity, audit: rec}
}

// CreateInput is the metadata for a new dataset draft.
type CreateInput struct {
	Title               string
	Description         string
	DataType            string
	Domain              string
	LicenseType         string
	SuggestedPriceCents *int64
	SourceDeclaration   *SourceDeclaration
}

// Create makes a draft dataset. The seller must be real-name verified (docs §2.2).
func (s *Service) Create(ctx context.Context, sellerID string, in CreateInput) (Dataset, error) {
	if err := s.requireVerified(ctx, sellerID); err != nil {
		return Dataset{}, err
	}
	if err := validateMeta(in.Title, in.DataType, in.LicenseType, in.SuggestedPriceCents); err != nil {
		return Dataset{}, err
	}
	d, err := s.repo.Create(ctx, Dataset{
		SellerID:            sellerID,
		Title:               strings.TrimSpace(in.Title),
		Description:         in.Description,
		DataType:            in.DataType,
		Domain:              in.Domain,
		LicenseType:         in.LicenseType,
		SuggestedPriceCents: in.SuggestedPriceCents,
		SourceDeclaration:   in.SourceDeclaration,
	})
	if err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: sellerID, Action: "dataset.create", ResourceType: "dataset", ResourceID: d.ID})
	return d, nil
}

// Update edits draft/rejected metadata; the caller must own the dataset.
func (s *Service) Update(ctx context.Context, userID, id string, in CreateInput) (Dataset, error) {
	d, err := s.ownedEditable(ctx, userID, id)
	if err != nil {
		return Dataset{}, err
	}
	if err := validateMeta(in.Title, in.DataType, in.LicenseType, in.SuggestedPriceCents); err != nil {
		return Dataset{}, err
	}
	d.Title = strings.TrimSpace(in.Title)
	d.Description = in.Description
	d.DataType = in.DataType
	d.Domain = in.Domain
	d.LicenseType = in.LicenseType
	d.SuggestedPriceCents = in.SuggestedPriceCents
	if in.SourceDeclaration != nil {
		d.SourceDeclaration = in.SourceDeclaration
	}
	return s.repo.UpdateMeta(ctx, d)
}

// Get returns a dataset by id.
func (s *Service) Get(ctx context.Context, id string) (Dataset, error) {
	return s.repo.GetByID(ctx, id)
}

// ListMine returns the caller's datasets.
func (s *Service) ListMine(ctx context.Context, sellerID string, limit, offset int) ([]Dataset, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListBySeller(ctx, sellerID, limit, offset)
}

// SignSource records the seller's electronic signature on the source-legality
// declaration. The declaration must be present and commitment acknowledged.
func (s *Service) SignSource(ctx context.Context, userID, id string) (Dataset, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	if d.SellerID != userID {
		return Dataset{}, ErrForbidden
	}
	if d.SourceSignedAt != "" {
		return Dataset{}, ErrAlreadySigned
	}
	if d.SourceDeclaration == nil || !d.SourceDeclaration.Commitment {
		return Dataset{}, fmt.Errorf("%w: source declaration and commitment are required before signing", ErrValidation)
	}
	signed, err := s.repo.SignSource(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: userID, Action: "dataset.source_sign", ResourceType: "dataset", ResourceID: id,
		Detail: map[string]any{"contains_pii": d.SourceDeclaration.ContainsPII, "license_scope": d.SourceDeclaration.LicenseScope},
	})
	return signed, nil
}

func (s *Service) requireVerified(ctx context.Context, userID string) error {
	status, err := s.identity.KYCStatus(ctx, userID)
	if err != nil {
		return err
	}
	if status != kycVerified {
		return ErrNotVerified
	}
	return nil
}

func (s *Service) ownedEditable(ctx context.Context, userID, id string) (Dataset, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	if d.SellerID != userID {
		return Dataset{}, ErrForbidden
	}
	if d.Status != StatusDraft && d.Status != StatusRejected {
		return Dataset{}, ErrNotEditable
	}
	return d, nil
}

func validateMeta(title, dataType, license string, price *int64) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%w: title is required", ErrValidation)
	}
	switch dataType {
	case dataTypeText, dataTypeCode, dataTypeStructured:
	default:
		return fmt.Errorf("%w: data_type must be text, code or structured", ErrValidation)
	}
	switch license {
	case licenseCommercial, licenseResearch, licenseTrainOnly:
	default:
		return fmt.Errorf("%w: invalid license_type", ErrValidation)
	}
	if price != nil && *price < 0 {
		return fmt.Errorf("%w: price must be non-negative", ErrValidation)
	}
	return nil
}
