package dataset

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"path"
	"path/filepath"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/quality"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/metrics"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

var (
	ErrStorageUnavailable = errors.New("storage not configured")
	ErrUploadForbidden    = errors.New("upload does not belong to caller")
)

// UploadSession is returned to the client after init.
type UploadSession struct {
	UploadID  string `json:"upload_id"`
	ObjectKey string `json:"object_key"`
	PartSize  int64  `json:"suggested_part_size"`
}

const suggestedPartSize = 8 << 20 // 8 MiB

// InitUpload starts a multipart upload for a single data file of the dataset.
// The seller must own the dataset and have signed the source declaration
// (docs §2.2 — no upload before the legality commitment).
//
// MVP uploads proxy through the backend (Local driver). Production uses
// browser-direct presigned part URLs (OSS driver) — see storage/oss.go.
func (s *Service) InitUpload(ctx context.Context, userID, datasetID, filename string) (UploadSession, error) {
	if s.storage == nil {
		return UploadSession{}, ErrStorageUnavailable
	}
	d, err := s.repo.GetByID(ctx, datasetID)
	if err != nil {
		return UploadSession{}, err
	}
	if d.SellerID != userID {
		return UploadSession{}, ErrForbidden
	}
	if d.Status != StatusDraft && d.Status != StatusRejected && d.Status != StatusUploading {
		return UploadSession{}, ErrNotEditable
	}
	if d.SourceSignedAt == "" {
		return UploadSession{}, ErrNotSigned
	}

	objectKey := path.Join("datasets", datasetID, sanitizeFilename(filename))
	uploadID, err := s.storage.InitMultipart(ctx, objectKey)
	if err != nil {
		return UploadSession{}, err
	}
	if err := s.repo.SetStatus(ctx, datasetID, StatusUploading); err != nil {
		return UploadSession{}, err
	}
	return UploadSession{UploadID: uploadID, ObjectKey: objectKey, PartSize: suggestedPartSize}, nil
}

// UploadPart stores one part. Ownership is re-checked from the upload's target key.
func (s *Service) UploadPart(ctx context.Context, userID, uploadID string, partNumber int, r io.Reader) (int64, error) {
	if s.storage == nil {
		return 0, ErrStorageUnavailable
	}
	if _, err := s.ownUpload(ctx, userID, uploadID); err != nil {
		return 0, err
	}
	return s.storage.PutPart(ctx, uploadID, partNumber, r)
}

// CompleteUpload assembles the object, records a new version+file, and advances
// the dataset to "checking" so quality checks (PR-09) can run.
func (s *Service) CompleteUpload(ctx context.Context, userID, uploadID string) (Dataset, error) {
	if s.storage == nil {
		return Dataset{}, ErrStorageUnavailable
	}
	d, err := s.ownUpload(ctx, userID, uploadID)
	if err != nil {
		return Dataset{}, err
	}
	obj, err := s.storage.CompleteMultipart(ctx, uploadID)
	if err != nil {
		return Dataset{}, err
	}
	file := FileInput{
		ObjectKey:   obj.Key,
		SizeBytes:   obj.Size,
		SHA256:      obj.SHA256,
		ContentType: contentTypeOf(obj.Key),
	}

	// SimHash + quality checks are deferred to the quality queue (async worker
	// in production, inline in tests) so a large upload doesn't block the HTTP
	// response (docs §6.3). The dataset stays "checking" until the worker runs.
	versionID, err := s.repo.AddVersion(ctx, d.ID, obj.SHA256, "", file, StatusChecking)
	if err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: userID, Action: "dataset.upload_complete", ResourceType: "dataset", ResourceID: d.ID,
		Detail: map[string]any{"object_key": obj.Key, "size_bytes": obj.Size, "sha256": obj.SHA256},
	})

	s.enqueueQuality(qualityJob{DatasetID: d.ID, VersionID: versionID, ContentSHA256: obj.SHA256})
	return s.repo.GetByID(ctx, d.ID)
}

const maxScanBytes = 64 << 20 // 64 MiB cap for quality scanning

func (s *Service) readObject(ctx context.Context, key string) ([]byte, error) {
	rc, _, err := s.storage.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxScanBytes))
}

// processQuality reads the object, computes the SimHash, runs format/stats/PII/
// dedup, persists each result + sample count, and advances the dataset:
// pass/warn -> reviewing, any fail (e.g. undeclared PII) -> back to draft for
// the seller to fix (docs §6.3). Self-contained from the job so it can run in a
// background worker. Errors leave the dataset "checking" (retriable).
func (s *Service) processQuality(ctx context.Context, job qualityJob) error {
	d, err := s.repo.GetByID(ctx, job.DatasetID)
	if err != nil {
		return err
	}
	key, err := s.repo.CurrentObjectKey(ctx, job.DatasetID)
	if err != nil {
		return err
	}
	content, err := s.readObject(ctx, key)
	if err != nil {
		return err
	}

	if err := s.repo.SetVersionSimhash(ctx, job.VersionID, quality.SimHash(content)); err != nil {
		return err
	}

	declaredPII := d.SourceDeclaration != nil && d.SourceDeclaration.ContainsPII
	fmtChk := quality.Format(content, contentTypeOf(key))
	statsChk, sample := quality.Stats(content)
	piiChk := quality.PII(content, declaredPII)
	redactChk := quality.PIIRedaction(content)

	// Authenticity: prefer the PaperGuard sidecar for tabular data; fall back to
	// the in-process Go baseline whenever the sidecar is absent or errors, so a
	// score is always produced and the sidecar is never on the critical path.
	ct := contentTypeOf(key)
	authChk := quality.Authenticity(content, ct)
	if s.screener != nil && strings.Contains(ct, "csv") {
		if sc, err := s.screener.Screen(ctx, content, ct); err == nil {
			authChk = sc
		} else {
			slog.Warn("authenticity sidecar failed; using Go baseline", "dataset_id", d.ID, "err", err)
		}
	}

	dedupChk := quality.Check{Type: quality.TypeDedup, Result: quality.ResultPass, Report: map[string]any{}}
	if dup, err := s.repo.ContentDupExists(ctx, job.ContentSHA256, d.ID); err == nil && dup {
		dedupChk.Result = quality.ResultWarn
		dedupChk.Report["duplicate_of_existing_content"] = true
	}

	failed := false
	for _, chk := range []quality.Check{fmtChk, statsChk, piiChk, redactChk, authChk, dedupChk} {
		if err := s.repo.SaveQualityCheck(ctx, d.ID, job.VersionID, chk.Type, chk.Result, chk.Report); err != nil {
			return err
		}
		if chk.Result == quality.ResultFail {
			failed = true
		}
	}
	if err := s.repo.SetSampleCount(ctx, d.ID, sample); err != nil {
		return err
	}

	next := StatusReviewing
	if failed {
		next = StatusDraft // bounce back so the seller can de-identify / re-upload
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: d.SellerID, Action: "dataset.quality_checked", ResourceType: "dataset", ResourceID: d.ID,
		Detail: map[string]any{"passed": !failed, "next_status": next},
	})
	metrics.RecordQualityJob(next)
	return s.repo.SetStatus(ctx, d.ID, next)
}

// UploadStatus reports upload progress plus the dataset's current status.
func (s *Service) UploadStatus(ctx context.Context, userID, uploadID string) (storage.UploadStat, string, error) {
	if s.storage == nil {
		return storage.UploadStat{}, "", ErrStorageUnavailable
	}
	d, err := s.ownUpload(ctx, userID, uploadID)
	if err != nil {
		return storage.UploadStat{}, "", err
	}
	st, err := s.storage.Stat(ctx, uploadID)
	if err != nil {
		return storage.UploadStat{}, "", err
	}
	return st, d.Status, nil
}

// ownUpload resolves the dataset behind an upload id (its object key encodes the
// dataset id) and verifies the caller owns it.
func (s *Service) ownUpload(ctx context.Context, userID, uploadID string) (Dataset, error) {
	st, err := s.storage.Stat(ctx, uploadID)
	if err != nil {
		return Dataset{}, err
	}
	parts := strings.Split(st.ObjectKey, "/")
	if len(parts) < 2 || parts[0] != "datasets" {
		return Dataset{}, ErrUploadForbidden
	}
	d, err := s.repo.GetByID(ctx, parts[1])
	if err != nil {
		return Dataset{}, err
	}
	if d.SellerID != userID {
		return Dataset{}, ErrUploadForbidden
	}
	return d, nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "")
	if name == "" || name == "." || name == "/" {
		return "data.bin"
	}
	return name
}

func contentTypeOf(key string) string {
	if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
