package dataset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type fakeRepo struct {
	items map[string]Dataset
	seq   int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]Dataset{}} }

func (r *fakeRepo) Create(_ context.Context, d Dataset) (Dataset, error) {
	r.seq++
	d.ID = "ds-" + itoa(r.seq)
	d.Status = StatusDraft
	r.items[d.ID] = d
	return d, nil
}
func (r *fakeRepo) GetByID(_ context.Context, id string) (Dataset, error) {
	d, ok := r.items[id]
	if !ok {
		return Dataset{}, ErrNotFound
	}
	return d, nil
}
func (r *fakeRepo) UpdateMeta(_ context.Context, d Dataset) (Dataset, error) {
	if _, ok := r.items[d.ID]; !ok {
		return Dataset{}, ErrNotFound
	}
	r.items[d.ID] = d
	return d, nil
}
func (r *fakeRepo) ListBySeller(_ context.Context, sellerID string, _, _ int) ([]Dataset, error) {
	var out []Dataset
	for _, d := range r.items {
		if d.SellerID == sellerID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *fakeRepo) SignSource(_ context.Context, id string) (Dataset, error) {
	d := r.items[id]
	d.SourceSignedAt = "2026-01-01T00:00:00Z"
	r.items[id] = d
	return d, nil
}
func (r *fakeRepo) SetStatus(_ context.Context, id, status string) error {
	d, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	d.Status = status
	r.items[id] = d
	return nil
}
func (r *fakeRepo) AddVersion(_ context.Context, datasetID, contentSHA256, _ string, f FileInput, newStatus string) (string, error) {
	d, ok := r.items[datasetID]
	if !ok {
		return "", ErrNotFound
	}
	d.Status = newStatus
	d.TotalSizeBytes = f.SizeBytes
	d.CurrentVersionID = "ver-1"
	r.items[datasetID] = d
	return "ver-1", nil
}
func (r *fakeRepo) SaveQualityCheck(_ context.Context, _, _, _, _ string, _ any) error { return nil }
func (r *fakeRepo) ContentDupExists(_ context.Context, _, _ string) (bool, error)      { return false, nil }
func (r *fakeRepo) SetSampleCount(_ context.Context, id string, n int64) error {
	d := r.items[id]
	d.SampleCount = n
	r.items[id] = d
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type fakeIdentity struct{ status map[string]string }

func (f fakeIdentity) KYCStatus(_ context.Context, userID string) (string, error) {
	return f.status[userID], nil
}

func newSvc(verified ...string) *Service {
	id := fakeIdentity{status: map[string]string{}}
	for _, u := range verified {
		id.status[u] = kycVerified
	}
	return NewService(newFakeRepo(), id, nil)
}

func validDecl() *SourceDeclaration {
	return &SourceDeclaration{Source: "internal", CollectionMethod: "scrape", LicenseScope: "commercial", Commitment: true}
}

func TestCreateRequiresVerifiedSeller(t *testing.T) {
	ctx := context.Background()
	svc := newSvc() // nobody verified
	_, err := svc.Create(ctx, "u1", CreateInput{Title: "t", DataType: "text", LicenseType: "commercial"})
	if !errors.Is(err, ErrNotVerified) {
		t.Fatalf("want ErrNotVerified, got %v", err)
	}

	svc = newSvc("u1")
	d, err := svc.Create(ctx, "u1", CreateInput{Title: "中文语料", DataType: "text", LicenseType: "commercial", SourceDeclaration: validDecl()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.Status != StatusDraft || d.SellerID != "u1" {
		t.Fatalf("unexpected dataset: %+v", d)
	}
}

func TestCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newSvc("u1")
	bad := []CreateInput{
		{Title: "", DataType: "text", LicenseType: "commercial"},
		{Title: "t", DataType: "image", LicenseType: "commercial"},
		{Title: "t", DataType: "text", LicenseType: "weird"},
	}
	for i, in := range bad {
		if _, err := svc.Create(ctx, "u1", in); !errors.Is(err, ErrValidation) {
			t.Errorf("case %d: want ErrValidation, got %v", i, err)
		}
	}
}

func TestUpdateOwnershipAndState(t *testing.T) {
	ctx := context.Background()
	svc := newSvc("owner")
	d, _ := svc.Create(ctx, "owner", CreateInput{Title: "t", DataType: "text", LicenseType: "commercial", SourceDeclaration: validDecl()})

	// Non-owner cannot edit.
	if _, err := svc.Update(ctx, "intruder", d.ID, CreateInput{Title: "x", DataType: "text", LicenseType: "commercial"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	// Owner can edit a draft.
	if _, err := svc.Update(ctx, "owner", d.ID, CreateInput{Title: "新标题", DataType: "code", LicenseType: "research"}); err != nil {
		t.Fatalf("owner update: %v", err)
	}
}

func TestUploadFlow(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	id := fakeIdentity{status: map[string]string{"owner": kycVerified}}
	svc := NewService(newFakeRepo(), id, nil, WithStorage(store))

	d, _ := svc.Create(ctx, "owner", CreateInput{Title: "t", DataType: "text", LicenseType: "commercial", SourceDeclaration: validDecl()})

	// Cannot upload before signing the source declaration.
	if _, err := svc.InitUpload(ctx, "owner", d.ID, "data.txt"); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("want ErrNotSigned, got %v", err)
	}
	if _, err := svc.SignSource(ctx, "owner", d.ID); err != nil {
		t.Fatalf("sign: %v", err)
	}

	sess, err := svc.InitUpload(ctx, "owner", d.ID, "data.txt")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	body := "line1\nline2\nline3\n"
	if _, err := svc.UploadPart(ctx, "owner", sess.UploadID, 1, strings.NewReader(body)); err != nil {
		t.Fatalf("upload part: %v", err)
	}
	// Another user cannot drive this upload.
	if _, err := svc.UploadPart(ctx, "intruder", sess.UploadID, 2, strings.NewReader("x")); !errors.Is(err, ErrUploadForbidden) {
		t.Fatalf("want ErrUploadForbidden, got %v", err)
	}

	done, err := svc.CompleteUpload(ctx, "owner", sess.UploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Clean content passes all checks -> advances to reviewing.
	if done.Status != StatusReviewing {
		t.Fatalf("status = %q, want reviewing", done.Status)
	}
	if done.TotalSizeBytes != int64(len(body)) {
		t.Fatalf("size = %d, want %d", done.TotalSizeBytes, len(body))
	}
	if done.SampleCount != 3 {
		t.Fatalf("sample_count = %d, want 3", done.SampleCount)
	}
}

func TestUploadWithUndeclaredPIIBouncesToDraft(t *testing.T) {
	ctx := context.Background()
	store, _ := storage.NewLocal(t.TempDir())
	id := fakeIdentity{status: map[string]string{"owner": kycVerified}}
	svc := NewService(newFakeRepo(), id, nil, WithStorage(store))

	// Declaration says no PII (contains_pii=false).
	d, _ := svc.Create(ctx, "owner", CreateInput{Title: "t", DataType: "text", LicenseType: "commercial", SourceDeclaration: validDecl()})
	_, _ = svc.SignSource(ctx, "owner", d.ID)
	sess, _ := svc.InitUpload(ctx, "owner", d.ID, "data.txt")
	_, _ = svc.UploadPart(ctx, "owner", sess.UploadID, 1, strings.NewReader("联系电话 13800138000 身份证 11010119900101123X\n"))

	done, err := svc.CompleteUpload(ctx, "owner", sess.UploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Undeclared PII is a hard fail -> bounced back to draft.
	if done.Status != StatusDraft {
		t.Fatalf("status = %q, want draft (PII bounce)", done.Status)
	}
}

func TestSignSourceFlow(t *testing.T) {
	ctx := context.Background()
	svc := newSvc("owner")

	// Without a declaration/commitment, signing is rejected.
	d, _ := svc.Create(ctx, "owner", CreateInput{Title: "t", DataType: "text", LicenseType: "commercial"})
	if _, err := svc.SignSource(ctx, "owner", d.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation without commitment, got %v", err)
	}

	// With a valid declaration, signing succeeds and is idempotent-guarded.
	d2, _ := svc.Create(ctx, "owner", CreateInput{Title: "t2", DataType: "text", LicenseType: "commercial", SourceDeclaration: validDecl()})
	signed, err := svc.SignSource(ctx, "owner", d2.ID)
	if err != nil || signed.SourceSignedAt == "" {
		t.Fatalf("sign: %v signedAt=%q", err, signed.SourceSignedAt)
	}
	if _, err := svc.SignSource(ctx, "owner", d2.ID); !errors.Is(err, ErrAlreadySigned) {
		t.Fatalf("want ErrAlreadySigned, got %v", err)
	}
}
