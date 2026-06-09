package dataset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/quality"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// IdentityChecker lets the dataset module ask the identity module (auth)
// whether a user is real-name verified, without importing auth or touching the
// users table (modular-monolith boundary).
type IdentityChecker interface {
	KYCStatus(ctx context.Context, userID string) (string, error)
}

// qualityJob is a unit of deferred quality work for one uploaded version.
type qualityJob struct {
	DatasetID     string
	VersionID     string
	ContentSHA256 string
}

// Service holds dataset business logic.
type Service struct {
	repo     Repository
	identity IdentityChecker
	audit    audit.Recorder
	storage  storage.Storage

	// Quality queue: when qCh is non-nil, upload-complete enqueues quality
	// checks to background workers (async); otherwise they run inline (used in
	// tests for determinism). The interface is queue-agnostic — swap the
	// in-process worker for Asynq/Redis at scale without touching call sites.
	qCh chan qualityJob
	wg  sync.WaitGroup

	// Optional PaperGuard authenticity sidecar. When nil, processQuality uses
	// the in-process Go baseline (quality.Authenticity).
	screener authenticityScreener

	// Optional notification emitter. When set, quality completion triggers
	// a seller notification.
	notifier QualityNotifier

	// Optional cert registrar for public verification.
	certReg CertRegistrar
}

// QualityNotifier emits a notification when quality checks finish.
type QualityNotifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

// CertRegistrar persists a certificate idempotently for public lookup.
type CertRegistrar interface {
	Register(ctx context.Context, certID, resourceType, resourceID string) error
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithStorage wires the object store used by the upload endpoints.
func WithStorage(s storage.Storage) Option { return func(svc *Service) { svc.storage = s } }

// WithAuthenticitySidecar points the quality worker at the PaperGuard
// authenticity sidecar at baseURL. When set, tabular datasets are screened by
// the sidecar; on any sidecar error the worker falls back to the Go baseline.
func WithAuthenticitySidecar(baseURL string, timeout time.Duration) Option {
	return func(svc *Service) { svc.screener = newHTTPScreener(baseURL, timeout) }
}

// WithAsyncQuality starts `workers` background goroutines draining a buffered
// queue so quality checks don't block the upload response. Call Close on
// shutdown to drain in-flight jobs.
func WithAsyncQuality(workers, buffer int) Option {
	return func(svc *Service) {
		if workers < 1 {
			workers = 1
		}
		if buffer < 1 {
			buffer = 1
		}
		svc.qCh = make(chan qualityJob, buffer)
		for i := 0; i < workers; i++ {
			svc.wg.Add(1)
			go func() {
				defer svc.wg.Done()
				for job := range svc.qCh {
					if err := svc.processQuality(context.Background(), job); err != nil {
						slog.Error("quality job failed", "dataset_id", job.DatasetID, "version_id", job.VersionID, "err", err)
					}
				}
			}()
		}
	}
}

func NewService(repo Repository, identity IdentityChecker, rec audit.Recorder, opts ...Option) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	s := &Service{repo: repo, identity: identity, audit: rec}
	for _, o := range opts {
		o(s)
	}
	return s
}

// enqueueQuality dispatches a quality job: to the worker pool if async is
// enabled, otherwise inline (synchronous) so callers/tests see the result
// immediately.
func (s *Service) enqueueQuality(job qualityJob) {
	if s.qCh != nil {
		s.qCh <- job
		return
	}
	if err := s.processQuality(context.Background(), job); err != nil {
		slog.Error("inline quality job failed", "dataset_id", job.DatasetID, "err", err)
	}
}

// SetQualityNotifier wires the notification emitter so quality completion
// sends a seller notification. Optional (may be nil in tests).
func (s *Service) SetQualityNotifier(n QualityNotifier) { s.notifier = n }

// SetCertRegistrar wires the cert registrar so dataset certificates are
// registered for public lookup.
func (s *Service) SetCertRegistrar(r CertRegistrar) { s.certReg = r }

// Close drains and stops the quality workers (no-op if async wasn't enabled).
func (s *Service) Close() {
	if s.qCh != nil {
		close(s.qCh)
		s.wg.Wait()
	}
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

// UpdateDatasheet sets a dataset's structured documentation. Unlike core
// metadata, the datasheet is documentation and may be edited by the owner at any
// lifecycle stage (including after publish). A nil datasheet clears it.
func (s *Service) UpdateDatasheet(ctx context.Context, userID, id string, ds *Datasheet) (Dataset, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Dataset{}, err
	}
	if d.SellerID != userID {
		return Dataset{}, ErrForbidden
	}
	if ds != nil && ds.isEmpty() {
		ds = nil // treat an all-blank submission as clearing the datasheet
	}
	return s.repo.SetDatasheet(ctx, id, ds)
}

// Get returns a dataset by id.
func (s *Service) Get(ctx context.Context, id string) (Dataset, error) {
	return s.repo.GetByID(ctx, id)
}

// Versions returns a dataset's version history (newest first).
func (s *Service) Versions(ctx context.Context, id string) ([]VersionInfo, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	vs, err := s.repo.ListVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	if vs == nil {
		vs = []VersionInfo{}
	}
	return vs, nil
}

// QualityReport returns the buyer-facing quality checks for a dataset's current
// version. Read-only and transparency-oriented: the persisted reports carry only
// counts/scores/metadata (no raw personal data), so they are safe to surface.
func (s *Service) QualityReport(ctx context.Context, id string) ([]QualityCheck, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	checks, err := s.repo.ListQualityChecks(ctx, id)
	if err != nil {
		return nil, err
	}
	if checks == nil {
		checks = []QualityCheck{}
	}
	return checks, nil
}

// Certificate returns the dataset's integrity & registration certificate.
func (s *Service) Certificate(ctx context.Context, id string) (map[string]any, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	vm, err := s.repo.CurrentVersionMeta(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	checks, err := s.repo.ListQualityChecks(ctx, id)
	if err != nil {
		return nil, err
	}
	cert := BuildCertificate(d, vm, checks)
	// Register for public verification (best-effort, non-blocking).
	if s.certReg != nil {
		if cid, ok := cert["certificate_id"].(string); ok && cid != "" {
			_ = s.certReg.Register(ctx, cid, "dataset", d.ID)
		}
	}
	return cert, nil
}

// CroissantMetadata returns the dataset's MLCommons Croissant 1.0 JSON-LD — a
// machine-readable description usable by Croissant-aware ML loaders and dataset
// search. baseURL is the public site origin (e.g. https://host).
func (s *Service) CroissantMetadata(ctx context.Context, id, baseURL string) (map[string]any, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	vm, err := s.repo.CurrentVersionMeta(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	checks, err := s.repo.ListQualityChecks(ctx, id)
	if err != nil {
		return nil, err
	}
	return BuildCroissant(d, vm, checks, baseURL), nil
}

// Purchasable is the purchase-relevant view of a dataset (consumed by the order
// module via its own interface, so order never imports dataset internals).
type Purchasable struct {
	SellerID   string
	VersionID  string
	PriceCents int64
	Published  bool
}

// ForPurchase returns purchase info: effective price (final overrides
// suggested), current version, and whether it is published.
func (s *Service) ForPurchase(ctx context.Context, id string) (Purchasable, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Purchasable{}, err
	}
	var price int64
	switch {
	case d.FinalPriceCents != nil:
		price = *d.FinalPriceCents
	case d.SuggestedPriceCents != nil:
		price = *d.SuggestedPriceCents
	}
	return Purchasable{
		SellerID:   d.SellerID,
		VersionID:  d.CurrentVersionID,
		PriceCents: price,
		Published:  d.Status == StatusPublished,
	}, nil
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

// CurrentObjectKey returns the object key of the dataset's current version file
// (consumed by the delivery module via its own interface).
func (s *Service) CurrentObjectKey(ctx context.Context, datasetID string) (string, error) {
	return s.repo.CurrentObjectKey(ctx, datasetID)
}

// List returns published datasets matching the filter (browse/search).
func (s *Service) List(ctx context.Context, f ListFilter) ([]Dataset, error) {
	return s.repo.ListPublished(ctx, f)
}

// SearchPublished is the search-module adapter: same as List but named for the
// search.DatasetSearcher interface so the server can bridge without the search
// package importing dataset internals.
func (s *Service) SearchPublished(ctx context.Context, f ListFilter) ([]Dataset, error) {
	return s.repo.ListPublished(ctx, f)
}

// AdminListByStatus powers ops queues (e.g. status=reviewing). Ops-gated at the
// router; the status must be a known lifecycle state.
func (s *Service) AdminListByStatus(ctx context.Context, status string, limit, offset int) ([]Dataset, error) {
	switch status {
	case StatusDraft, StatusUploading, StatusChecking, StatusReviewing, StatusPublished, StatusRejected, StatusDelisted:
	default:
		return nil, fmt.Errorf("%w: unknown status %q", ErrValidation, status)
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByStatus(ctx, status, limit, offset)
}

// PreviewResult is a limited, PII-masked sample for the detail page.
type PreviewResult struct {
	Lines       []string `json:"lines"`
	LineCount   int      `json:"line_count"`
	SampleCount int64    `json:"dataset_sample_count"`
	Truncated   bool     `json:"truncated"`
}

const (
	previewMaxBytes = 64 << 10 // read at most 64KiB for a preview
	previewMaxLines = 20       // show at most 20 sampled lines
	previewLineCap  = 500      // truncate each line to 500 chars
)

// Preview returns a masked, capped sample of a published dataset (docs §6.4):
// limited rows, PII masked, long lines truncated. Rate-limited at the router.
func (s *Service) Preview(ctx context.Context, datasetID string) (PreviewResult, error) {
	d, err := s.repo.GetByID(ctx, datasetID)
	if err != nil {
		return PreviewResult{}, err
	}
	if d.Status != StatusPublished {
		return PreviewResult{}, ErrNotFound // only published datasets are previewable
	}
	if s.storage == nil {
		return PreviewResult{}, ErrStorageUnavailable
	}
	key, err := s.repo.CurrentObjectKey(ctx, datasetID)
	if err != nil {
		return PreviewResult{}, err
	}
	rc, _, err := s.storage.Open(ctx, key)
	if err != nil {
		return PreviewResult{}, err
	}
	defer rc.Close()
	buf, err := io.ReadAll(io.LimitReader(rc, previewMaxBytes))
	if err != nil {
		return PreviewResult{}, err
	}

	raw := strings.Split(string(buf), "\n")
	lines := make([]string, 0, previewMaxLines)
	for _, ln := range raw {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		masked := quality.MaskPII(ln)
		if len(masked) > previewLineCap {
			masked = masked[:previewLineCap] + "…"
		}
		lines = append(lines, masked)
		if len(lines) >= previewMaxLines {
			break
		}
	}
	return PreviewResult{
		Lines:       lines,
		LineCount:   len(lines),
		SampleCount: d.SampleCount,
		Truncated:   int64(len(lines)) < d.SampleCount,
	}, nil
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
