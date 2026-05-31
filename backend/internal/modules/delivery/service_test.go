package delivery

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type fakeRepo struct{ grants map[string]*Grant } // by token hash

func newFakeRepo() *fakeRepo { return &fakeRepo{grants: map[string]*Grant{}} }

func (r *fakeRepo) Upsert(_ context.Context, orderID, tokenHash string, exp time.Time, max int, fp string) error {
	// Drop any prior grant for this order.
	for h, g := range r.grants {
		if g.OrderID == orderID {
			delete(r.grants, h)
		}
	}
	r.grants[tokenHash] = &Grant{ID: "d1", OrderID: orderID, ExpiresAt: exp, MaxDownloads: max, Fingerprint: fp}
	return nil
}
func (r *fakeRepo) GetByTokenHash(_ context.Context, h string) (Grant, error) {
	g, ok := r.grants[h]
	if !ok {
		return Grant{}, ErrTokenInvalid
	}
	return *g, nil
}
func (r *fakeRepo) ConsumeDownload(_ context.Context, id, _ string) (bool, error) {
	for _, g := range r.grants {
		if g.ID == id {
			if g.DownloadCount >= g.MaxDownloads {
				return false, nil
			}
			g.DownloadCount++
			return true, nil
		}
	}
	return false, nil
}

type fakeOrders struct {
	info     OrderInfo
	delivers int
}

func (f *fakeOrders) GetSystem(_ context.Context, _ string) (OrderInfo, error) { return f.info, nil }
func (f *fakeOrders) MarkDelivered(_ context.Context, _ string) error {
	f.delivers++
	f.info.Status = "delivered"
	return nil
}

type fakeDatasets struct{ key string }

func (f fakeDatasets) CurrentObjectKey(_ context.Context, _ string) (string, error) {
	return f.key, nil
}

func setup(t *testing.T, status string) (*Service, *fakeOrders, storage.Storage, string) {
	t.Helper()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	key := "datasets/ds1/data.txt"
	up, _ := store.InitMultipart(context.Background(), key)
	_, _ = store.PutPart(context.Background(), up, 1, readerOf("买到的训练数据内容\n"))
	if _, err := store.CompleteMultipart(context.Background(), up); err != nil {
		t.Fatalf("seed object: %v", err)
	}
	orders := &fakeOrders{info: OrderInfo{ID: "o1", BuyerID: "buyer", Status: status, DatasetID: "ds1"}}
	svc := NewService(newFakeRepo(), orders, fakeDatasets{key: key}, store, "fp-secret", nil)
	return svc, orders, store, key
}

func readerOf(s string) io.Reader { return &strReader{s: s} }

type strReader struct {
	s string
	i int
}

func (r *strReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

func TestRequestDownloadGuards(t *testing.T) {
	ctx := context.Background()

	svc, _, _, _ := setup(t, "paid")
	if _, _, err := svc.RequestDownload(ctx, "intruder", "o1", true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if _, _, err := svc.RequestDownload(ctx, "buyer", "o1", false); !errors.Is(err, ErrLicenseRequired) {
		t.Fatalf("want ErrLicenseRequired, got %v", err)
	}

	svc2, _, _, _ := setup(t, "created")
	if _, _, err := svc2.RequestDownload(ctx, "buyer", "o1", true); !errors.Is(err, ErrNotPaid) {
		t.Fatalf("want ErrNotPaid, got %v", err)
	}
}

func TestDownloadFlow(t *testing.T) {
	ctx := context.Background()
	svc, orders, _, _ := setup(t, "paid")

	token, _, err := svc.RequestDownload(ctx, "buyer", "o1", true)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if orders.delivers != 1 || orders.info.Status != "delivered" {
		t.Fatalf("paid order should be marked delivered once, got %d status=%s", orders.delivers, orders.info.Status)
	}

	// Download up to the quota.
	for i := 0; i < maxDownloads; i++ {
		res, err := svc.Download(ctx, token, "1.2.3.4")
		if err != nil {
			t.Fatalf("download %d: %v", i, err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(b) != "买到的训练数据内容\n" {
			t.Fatalf("content mismatch: %q", b)
		}
	}
	// Quota exhausted.
	if _, err := svc.Download(ctx, token, "1.2.3.4"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid after quota, got %v", err)
	}
	// Unknown token.
	if _, err := svc.Download(ctx, "deadbeef", "1.2.3.4"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid for unknown token, got %v", err)
	}
}

func TestDownloadExpired(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setup(t, "paid")
	token, _, _ := svc.RequestDownload(ctx, "buyer", "o1", true)

	// Jump past expiry.
	svc.nowFn = func() time.Time { return time.Now().Add(tokenTTL + time.Minute) }
	if _, err := svc.Download(ctx, token, "1.2.3.4"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid for expired token, got %v", err)
	}
}
