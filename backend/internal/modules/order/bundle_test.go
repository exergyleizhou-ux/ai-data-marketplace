package order

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
)

// --- fake implementations for testing BundleOrders ---

type fakeBundleStorage struct {
	files map[string][]byte
}

func (f *fakeBundleStorage) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	if b, ok := f.files[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
	}
	return nil, 0, fmt.Errorf("not found: %s", key)
}

type fakeBundleSource struct {
	keys      map[string]string
	filenames map[string]string
}

func (f *fakeBundleSource) CurrentObjectKey(_ context.Context, did string) (string, error) {
	if k, ok := f.keys[did]; ok {
		return k, nil
	}
	return "", fmt.Errorf("no key for %s", did)
}
func (f *fakeBundleSource) SuggestFilename(_ context.Context, did string) (string, error) {
	if fn, ok := f.filenames[did]; ok {
		return fn, nil
	}
	return "data_" + did[:8] + ".bin", nil
}

func newBundleSvc(orders []Order) *Service {
	repo := &fakeRepo{orders: map[string]Order{}}
	for _, o := range orders {
		repo.orders[o.ID] = o
	}
	svc := &Service{repo: repo}
	return svc
}

func makeOrder(id, buyerID, datasetID, status, productType string) Order {
	return Order{
		ID: id, BuyerID: buyerID, SellerID: "seller", DatasetID: datasetID,
		VersionID: "v1", LicenseType: "commercial",
		AmountCents: 1000, PlatformFeeCents: 100, SellerAmountCents: 900,
		Status: status, ProductType: productType,
	}
}

// --- tests ---

func TestBundleOrders_PacksAllSettledIntoValidZip(t *testing.T) {
	ctx := context.Background()
	svc := newBundleSvc([]Order{
		makeOrder("o1", "buyer", "d1", StatusSettled, ProductDownload),
		makeOrder("o2", "buyer", "d2", StatusSettled, ProductDownload),
	})
	svc.bundleSrc = &fakeBundleSource{
		keys:      map[string]string{"d1": "k1", "d2": "k2"},
		filenames: map[string]string{"d1": "file1.csv", "d2": "file2.json"},
	}
	svc.store = &fakeBundleStorage{
		files: map[string][]byte{"k1": []byte("hello"), "k2": []byte("world")},
	}

	var buf bytes.Buffer
	if err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1", "o2"}); err != nil {
		t.Fatalf("BundleOrders: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip entries = %d, want 2", len(zr.File))
	}
	if zr.File[0].Name != "file1.csv" {
		t.Errorf("entry 0 name = %q, want file1.csv", zr.File[0].Name)
	}
	if zr.File[1].Name != "file2.json" {
		t.Errorf("entry 1 name = %q, want file2.json", zr.File[1].Name)
	}
}

func TestBundleOrders_RejectsEmptyOrderIDs(t *testing.T) {
	svc := newBundleSvc(nil)
	var buf bytes.Buffer
	if err := svc.BundleOrders(context.Background(), &buf, "buyer", nil); err == nil {
		t.Fatal("empty order_ids must fail")
	}
}

func TestBundleOrders_RejectsMoreThan20(t *testing.T) {
	svc := newBundleSvc(nil)
	ids := make([]string, 21)
	for i := range ids {
		ids[i] = "o"
	}
	var buf bytes.Buffer
	if err := svc.BundleOrders(context.Background(), &buf, "buyer", ids); err == nil {
		t.Fatal("more than 20 order_ids must fail")
	}
}

func TestBundleOrders_RejectsForeignOrder(t *testing.T) {
	ctx := context.Background()
	svc := newBundleSvc([]Order{
		makeOrder("o1", "other-buyer", "d1", StatusSettled, ProductDownload),
	})
	svc.bundleSrc = &fakeBundleSource{
		keys:      map[string]string{"d1": "k1"},
		filenames: map[string]string{"d1": "f.csv"},
	}
	svc.store = &fakeBundleStorage{files: map[string][]byte{"k1": []byte("x")}}

	var buf bytes.Buffer
	err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1"})
	if err == nil {
		t.Fatal("foreign order must fail")
	}
	// buf must be untouched.
	if buf.Len() != 0 {
		t.Fatalf("buf.Len() = %d, want 0 (no zip bytes on preflight failure)", buf.Len())
	}
}

func TestBundleOrders_RejectsNonSettledOrder(t *testing.T) {
	ctx := context.Background()
	svc := newBundleSvc([]Order{
		makeOrder("o1", "buyer", "d1", StatusPaid, ProductDownload),
	})
	svc.bundleSrc = &fakeBundleSource{keys: map[string]string{"d1": "k1"}, filenames: map[string]string{"d1": "f.csv"}}
	svc.store = &fakeBundleStorage{files: map[string][]byte{"k1": []byte("x")}}

	var buf bytes.Buffer
	err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1"})
	if err == nil {
		t.Fatal("non-settled order must fail")
	}
}

func TestBundleOrders_RejectsComputeOrder(t *testing.T) {
	ctx := context.Background()
	svc := newBundleSvc([]Order{
		makeOrder("o1", "buyer", "d1", StatusSettled, ProductCompute),
	})
	svc.bundleSrc = &fakeBundleSource{keys: map[string]string{"d1": "k1"}, filenames: map[string]string{"d1": "f.csv"}}
	svc.store = &fakeBundleStorage{files: map[string][]byte{"k1": []byte("x")}}

	var buf bytes.Buffer
	err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1"})
	if err == nil {
		t.Fatal("compute order must fail")
	}
}

func TestBundleOrders_PreflightFailureDoesNotWriteZipBytes(t *testing.T) {
	ctx := context.Background()
	// Two orders, second belongs to another buyer.
	svc := newBundleSvc([]Order{
		makeOrder("o1", "buyer", "d1", StatusSettled, ProductDownload),
		makeOrder("o2", "other-buyer", "d2", StatusSettled, ProductDownload),
	})
	svc.bundleSrc = &fakeBundleSource{
		keys:      map[string]string{"d1": "k1", "d2": "k2"},
		filenames: map[string]string{"d1": "f1.csv", "d2": "f2.csv"},
	}
	svc.store = &fakeBundleStorage{files: map[string][]byte{"k1": []byte("x"), "k2": []byte("y")}}

	var buf bytes.Buffer
	err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1", "o2"})
	if err == nil {
		t.Fatal("second order foreign must fail")
	}
	if buf.Len() != 0 {
		t.Fatalf("buf.Len() = %d, want 0", buf.Len())
	}
}

func TestBundleOrders_StorageOpenFailureMidStreamReturnsError(t *testing.T) {
	ctx := context.Background()
	svc := newBundleSvc([]Order{
		makeOrder("o1", "buyer", "d1", StatusSettled, ProductDownload),
		makeOrder("o2", "buyer", "d2", StatusSettled, ProductDownload),
	})
	svc.bundleSrc = &fakeBundleSource{
		keys:      map[string]string{"d1": "k1", "d2": "k2"},
		filenames: map[string]string{"d1": "ok.csv", "d2": "missing.csv"},
	}
	svc.store = &fakeBundleStorage{
		files: map[string][]byte{"k1": []byte("ok-data")}, // k2 missing → Open error
	}

	var buf bytes.Buffer
	err := svc.BundleOrders(ctx, &buf, "buyer", []string{"o1", "o2"})
	if err == nil {
		t.Fatal("mid-stream Open failure must return error")
	}
	// The zip should still be closed (structurally valid with one entry).
	zr, zerr := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if zerr != nil {
		t.Fatalf("zip is still structurally valid after stream failure: %v", zerr)
	}
	if len(zr.File) != 1 {
		t.Fatalf("zip entries = %d, want 1 (first file succeeded, second failed)", len(zr.File))
	}
}
