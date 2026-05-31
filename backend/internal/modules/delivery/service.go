package delivery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// OrderInfo is the order data delivery needs.
type OrderInfo struct {
	ID        string
	BuyerID   string
	Status    string
	DatasetID string
}

// OrderGateway lets delivery read an order and mark it delivered (impl by order).
type OrderGateway interface {
	GetSystem(ctx context.Context, orderID string) (OrderInfo, error)
	MarkDelivered(ctx context.Context, orderID string) error
}

// DatasetReader resolves the object key of a dataset's current version (impl by dataset).
type DatasetReader interface {
	CurrentObjectKey(ctx context.Context, datasetID string) (string, error)
}

// Service issues download grants and streams delivered objects.
type Service struct {
	repo     Repository
	orders   OrderGateway
	datasets DatasetReader
	storage  storage.Storage
	audit    audit.Recorder
	fpSecret string
	nowFn    func() time.Time
}

func NewService(repo Repository, orders OrderGateway, datasets DatasetReader, store storage.Storage, fingerprintSecret string, rec audit.Recorder) *Service {
	if rec == nil {
		rec = audit.Noop{}
	}
	return &Service{repo: repo, orders: orders, datasets: datasets, storage: store, audit: rec, fpSecret: fingerprintSecret, nowFn: time.Now}
}

// RequestDownload verifies payment + license acceptance, advances the order to
// delivered on first request, and returns a one-time short-lived token.
func (s *Service) RequestDownload(ctx context.Context, buyerID, orderID string, licenseAgreed bool) (token string, expiresAt time.Time, err error) {
	o, err := s.orders.GetSystem(ctx, orderID)
	if err != nil {
		return "", time.Time{}, err
	}
	if o.BuyerID != buyerID {
		return "", time.Time{}, ErrForbidden
	}
	if !licenseAgreed {
		return "", time.Time{}, ErrLicenseRequired
	}
	switch o.Status {
	case "paid", "delivered", "confirmed":
	default:
		return "", time.Time{}, ErrNotPaid
	}
	if o.Status == "paid" {
		if err := s.orders.MarkDelivered(ctx, orderID); err != nil {
			return "", time.Time{}, err
		}
	}

	raw := randomToken()
	expiresAt = s.nowFn().Add(tokenTTL)
	fingerprint := fingerprint(s.fpSecret, buyerID, orderID)
	if err := s.repo.Upsert(ctx, orderID, hashToken(raw), expiresAt, maxDownloads, fingerprint); err != nil {
		return "", time.Time{}, err
	}
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "delivery.license_sign", ResourceType: "order", ResourceID: orderID})
	s.audit.Record(ctx, audit.Entry{ActorID: buyerID, Action: "delivery.request", ResourceType: "order", ResourceID: orderID,
		Detail: map[string]any{"fingerprint": fingerprint, "expires_at": expiresAt}})
	return raw, expiresAt, nil
}

// DownloadResult carries either a redirect URL (object storage hands out a
// short-lived presigned link — production best practice, bytes bypass the app)
// or a byte stream (Local driver). Exactly one is set.
type DownloadResult struct {
	RedirectURL string
	Body        io.ReadCloser
	Size        int64
}

// Download validates the token (expiry/quota), records the download (with
// fingerprint + IP for traceability), and streams the object.
func (s *Service) Download(ctx context.Context, token, ip string) (DownloadResult, error) {
	g, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		return DownloadResult{}, err
	}
	if s.nowFn().After(g.ExpiresAt) || g.DownloadCount >= g.MaxDownloads {
		return DownloadResult{}, ErrTokenInvalid
	}
	o, err := s.orders.GetSystem(ctx, g.OrderID)
	if err != nil {
		return DownloadResult{}, err
	}
	key, err := s.datasets.CurrentObjectKey(ctx, o.DatasetID)
	if err != nil {
		return DownloadResult{}, err
	}
	ok, err := s.repo.ConsumeDownload(ctx, g.ID, ip)
	if err != nil {
		return DownloadResult{}, err
	}
	if !ok {
		return DownloadResult{}, ErrTokenInvalid // raced to the quota / expiry
	}
	s.audit.Record(ctx, audit.Entry{ActorID: o.BuyerID, Action: "delivery.download", ResourceType: "order", ResourceID: o.ID, IP: ip,
		Detail: map[string]any{"fingerprint": g.Fingerprint, "object_key": key}})

	// If the driver can presign (S3/MinIO/OSS), hand out a short-lived direct
	// URL so the bytes don't transit the app server.
	if pg, ok := s.storage.(storage.PresignedGetter); ok {
		url, err := pg.PresignGet(ctx, key, tokenTTL)
		if err != nil {
			return DownloadResult{}, err
		}
		return DownloadResult{RedirectURL: url}, nil
	}

	rc, size, err := s.storage.Open(ctx, key)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Body: rc, Size: size}, nil
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// fingerprint binds a delivery to buyer+order for post-leak tracing (weak
// deterrent, docs §2.3) — recorded, not embedded in the bytes.
func fingerprint(secret, buyerID, orderID string) string {
	h := sha256.Sum256([]byte(secret + ":" + buyerID + ":" + orderID))
	return hex.EncodeToString(h[:])[:32]
}
