package dataset

import (
	"context"
	"errors"
	"io"
	"mime"
	"path"
	"path/filepath"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
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
	// simhash is computed by the quality stage (PR-09); left empty here.
	if _, err := s.repo.AddVersion(ctx, d.ID, obj.SHA256, "", file, StatusChecking); err != nil {
		return Dataset{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: userID, Action: "dataset.upload_complete", ResourceType: "dataset", ResourceID: d.ID,
		Detail: map[string]any{"object_key": obj.Key, "size_bytes": obj.Size, "sha256": obj.SHA256},
	})
	return s.repo.GetByID(ctx, d.ID)
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
