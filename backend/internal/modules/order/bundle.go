package order

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// BundleSource is injected by the server so order doesn't import dataset.
type BundleSource interface {
	CurrentObjectKey(ctx context.Context, datasetID string) (string, error)
	SuggestFilename(ctx context.Context, datasetID string) (string, error)
}

// BundleEntry is a ready-to-stream item after successful preflight.
type BundleEntry struct {
	Order Order
	Key   string
	Name  string
}

// BundlePreflight validates N settled download orders belonging to one buyer
// and resolves their object keys + zip filenames.  It does NOT touch w — the
// caller decides whether to set HTTP headers based on success.
func (s *Service) BundlePreflight(ctx context.Context, buyerID string, orderIDs []string) ([]BundleEntry, error) {
	if len(orderIDs) == 0 || len(orderIDs) > 20 {
		return nil, fmt.Errorf("%w: order_ids length must be 1–20", ErrValidation)
	}
	if s.bundleSrc == nil || s.store == nil {
		return nil, fmt.Errorf("%w: bundle not available (no storage)", ErrValidation)
	}
	entries := make([]BundleEntry, len(orderIDs))
	for i, oid := range orderIDs {
		o, err := s.repo.GetByID(ctx, oid)
		if err != nil {
			return nil, err
		}
		if o.BuyerID != buyerID {
			return nil, fmt.Errorf("%w: order %s does not belong to buyer", ErrForbidden, trunc8(oid))
		}
		if o.Status != StatusSettled {
			return nil, ErrBadTransition
		}
		if o.ProductType == ProductCompute {
			return nil, fmt.Errorf("%w: compute orders cannot be bundled", ErrValidation)
		}
		key, err := s.bundleSrc.CurrentObjectKey(ctx, o.DatasetID)
		if err != nil {
			return nil, fmt.Errorf("bundle: resolve key for %s: %w", oid, err)
		}
		name, err := s.bundleSrc.SuggestFilename(ctx, o.DatasetID)
		if err != nil {
			return nil, fmt.Errorf("bundle: suggest name for %s: %w", oid, err)
		}
		entries[i] = BundleEntry{Order: o, Key: key, Name: name}
	}
	return entries, nil
}

// BundleStream writes a zip archive of entries to w.  Caller must have already
// called BundlePreflight successfully.  If an Open fails mid-stream the error
// is returned but zw.Close() is still called so the zip is structurally valid
// up to the failure point.
func (s *Service) BundleStream(ctx context.Context, w io.Writer, entries []BundleEntry) error {
	zw := zip.NewWriter(w)
	var zwErr error
	sawOne := make(map[string]bool)
	for i, e := range entries {
		unique := e.Name
		if sawOne[unique] {
			unique = fmt.Sprintf("%s_%d", e.Name, i)
		}
		sawOne[unique] = true
		if zwErr != nil {
			continue
		}
		if err := streamZipEntry(ctx, s.store, zw, e.Key, unique); err != nil {
			slog.Warn("bundle stream failed", "order", e.Order.ID, "key", e.Key, "err", err)
			zwErr = err
		}
	}
	if cerr := zw.Close(); cerr != nil && zwErr == nil {
		zwErr = cerr
	}
	return zwErr
}

func streamZipEntry(ctx context.Context, opener fileOpener, zw *zip.Writer, key, name string) error {
	rc, sz, err := opener.Open(ctx, key)
	if err != nil {
		return fmt.Errorf("open %s: %w", key, err)
	}
	defer rc.Close()

	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
	if sz > 0 {
		hdr.UncompressedSize64 = uint64(sz)
	}
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := io.Copy(w, rc); err != nil {
		return fmt.Errorf("copy %s: %w", name, err)
	}
	return nil
}

// fileOpener abstracts storage.Open so tests can inject a fake.
type fileOpener interface {
	Open(ctx context.Context, objectKey string) (io.ReadCloser, int64, error)
}
