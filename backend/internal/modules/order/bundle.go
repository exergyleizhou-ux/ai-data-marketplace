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

// BundleOrders validates N settled download orders belonging to one buyer,
// then streams a zip archive of their object files to w.  Pre-flight checks
// run BEFORE the first zip byte is written so a validation error leaves w
// untouched.
func (s *Service) BundleOrders(ctx context.Context, w io.Writer, buyerID string, orderIDs []string) error {
	if len(orderIDs) == 0 || len(orderIDs) > 20 {
		return fmt.Errorf("%w: order_ids length must be 1–20", ErrValidation)
	}
	if s.bundleSrc == nil {
		return fmt.Errorf("%w: bundle not available (no storage)", ErrValidation)
	}

	// Pre-flight: load every order, verify ownership + status + product type.
	type entry struct {
		order Order
		key   string
		name  string
	}
	entries := make([]entry, len(orderIDs))
	for i, oid := range orderIDs {
		o, err := s.repo.GetByID(ctx, oid)
		if err != nil {
			return err
		}
		if o.BuyerID != buyerID {
			return fmt.Errorf("%w: order %s does not belong to buyer", ErrForbidden, trunc8(oid))
		}
		if o.Status != StatusSettled {
			return ErrBadTransition
		}
		if o.ProductType == ProductCompute {
			return fmt.Errorf("%w: compute orders cannot be bundled", ErrValidation)
		}
		key, err := s.bundleSrc.CurrentObjectKey(ctx, o.DatasetID)
		if err != nil {
			return fmt.Errorf("bundle: resolve key for %s: %w", oid, err)
		}
		name, err := s.bundleSrc.SuggestFilename(ctx, o.DatasetID)
		if err != nil {
			return fmt.Errorf("bundle: suggest name for %s: %w", oid, err)
		}
		entries[i] = entry{order: o, key: key, name: name}
	}

	// Stream the zip.  If an Open fails mid-stream we return the error but
	// still call zw.Close() so the received zip is at least structurally valid
	// up to that point.
	zw := zip.NewWriter(w)
	var zwErr error
	sawOne := make(map[string]bool)
	for i, e := range entries {
		unique := e.name
		if sawOne[unique] {
			unique = fmt.Sprintf("%s_%d", e.name, i)
		}
		sawOne[unique] = true
		if zwErr != nil {
			continue // don't write more entries after first failure
		}
		if err := streamZipEntry(ctx, s.store, zw, e.key, unique); err != nil {
			slog.Warn("bundle stream failed", "order", e.order.ID, "key", e.key, "err", err)
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
