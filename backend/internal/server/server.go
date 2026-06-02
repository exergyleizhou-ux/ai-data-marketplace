// Package server constructs the HTTP engine and wires module routers.
//
// Architectural rule (modular monolith): modules expose a router-registration
// function; the server composes them here. Modules MUST NOT reach into each
// other's packages or tables directly — only through exported interfaces.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/compute"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/dataset"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/delivery"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/order"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/payment"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/metrics"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	redispkg "github.com/lei/ai-data-marketplace/backend/internal/platform/redis"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type Server struct {
	cfg     *config.Config
	db      *pgxpool.Pool
	engine  *gin.Engine
	closers []func() // background components to drain on shutdown
}

// Close drains background components (e.g. the async quality workers). Call it
// after the HTTP server has stopped accepting requests.
func (s *Server) Close() {
	for _, c := range s.closers {
		c()
	}
}

// New builds the server. db may be nil in tests that exercise only routes that
// don't touch the database.
func New(cfg *config.Config, db *pgxpool.Pool) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	// CORS first (handles preflight); RequestID so logger/recovery correlate;
	// metrics times the whole handler stack.
	engine.Use(
		middleware.CORS(cfg.CORSAllowOrigin),
		middleware.RequestID(),
		metrics.Middleware(),
		middleware.Logger(),
		middleware.Recovery(),
	)

	s := &Server{cfg: cfg, db: db, engine: engine}
	s.routes()
	return s
}

// Handler exposes the underlying http.Handler for the server runner and tests.
func (s *Server) Handler() http.Handler { return s.engine }

// limiter returns a shared Redis-backed limiter, falling back to an in-memory
// one if Redis is unreachable so a Redis outage degrades to single-instance
// limiting rather than a hard failure.
func (s *Server) limiter() ratelimit.Limiter {
	if client, err := redispkg.New(context.Background(), s.cfg.RedisURL); err == nil {
		slog.Info("rate limiter backend", "type", "redis")
		return ratelimit.NewRedis(client)
	} else {
		slog.Warn("redis unavailable; using in-memory rate limiter", "err", err)
	}
	return ratelimit.NewInMemory()
}

// denylist returns a shared Redis-backed refresh-token denylist, falling back
// to an in-memory one when Redis is unreachable (same degradation as limiter).
func (s *Server) denylist() auth.Denylist {
	if client, err := redispkg.New(context.Background(), s.cfg.RedisURL); err == nil {
		slog.Info("token denylist backend", "type", "redis")
		return auth.NewRedisDenylist(client)
	}
	slog.Warn("redis unavailable; using in-memory token denylist")
	return auth.NewInMemoryDenylist()
}

// datasetPurchaseAdapter bridges dataset.Service to the order module's
// DatasetReader interface, converting between the two packages' value types so
// neither imports the other.
type datasetPurchaseAdapter struct{ ds *dataset.Service }

func (a datasetPurchaseAdapter) ForPurchase(ctx context.Context, id string) (order.Purchasable, error) {
	p, err := a.ds.ForPurchase(ctx, id)
	if err != nil {
		return order.Purchasable{}, err
	}
	return order.Purchasable{
		SellerID:   p.SellerID,
		VersionID:  p.VersionID,
		PriceCents: p.PriceCents,
		Published:  p.Published,
	}, nil
}

// stripePayoutStore bridges auth.Service to payment.PayoutAccountStore so the
// Stripe provider can persist connected-account ids without importing auth. It
// translates auth's not-found sentinel into the store contract's empty string.
type stripePayoutStore struct{ auth *auth.Service }

func (s stripePayoutStore) PayoutAccountRef(ctx context.Context, sellerID, channel string) (string, error) {
	ref, err := s.auth.PayoutAccountRef(ctx, sellerID, channel)
	if errors.Is(err, auth.ErrPayoutAccountNotFound) {
		return "", nil
	}
	return ref, err
}

func (s stripePayoutStore) SavePayoutAccount(ctx context.Context, sellerID, channel, accountRef string) error {
	return s.auth.SavePayoutAccount(ctx, sellerID, channel, accountRef)
}

// orderPaymentAdapter bridges order.Service to payment.OrderGateway, converting
// value types so neither package imports the other.
type orderPaymentAdapter struct{ o *order.Service }

func (a orderPaymentAdapter) GetSystem(ctx context.Context, id string) (payment.OrderInfo, error) {
	o, err := a.o.GetSystem(ctx, id)
	if err != nil {
		return payment.OrderInfo{}, err
	}
	return payment.OrderInfo{
		ID: o.ID, BuyerID: o.BuyerID, SellerID: o.SellerID, Status: o.Status,
		AmountCents: o.AmountCents, PlatformFeeCents: o.PlatformFeeCents, SellerAmountCents: o.SellerAmountCents,
	}, nil
}
func (a orderPaymentAdapter) MarkPaid(ctx context.Context, id string) error {
	_, err := a.o.MarkPaid(ctx, id)
	return err
}
func (a orderPaymentAdapter) MarkSettled(ctx context.Context, id string) error {
	_, err := a.o.MarkSettled(ctx, id)
	return err
}

// objectStorage builds the configured object-storage driver. Returns nil (and
// logs) on failure so upload endpoints degrade to "storage unavailable" rather
// than crashing the whole server.
func (s *Server) objectStorage() storage.Storage {
	switch s.cfg.StorageDriver {
	case "s3":
		store, err := storage.NewS3(context.Background(), storage.S3Config{
			Endpoint:  s.cfg.S3Endpoint,
			Bucket:    s.cfg.S3Bucket,
			AccessKey: s.cfg.S3AccessKey,
			SecretKey: s.cfg.S3SecretKey,
			UseSSL:    s.cfg.S3UseSSL,
			Region:    s.cfg.S3Region,
		})
		if err != nil {
			slog.Error("failed to init S3 storage", "err", err)
			return nil
		}
		slog.Info("object storage backend", "type", "s3", "endpoint", s.cfg.S3Endpoint, "bucket", s.cfg.S3Bucket)
		return store
	default:
		store, err := storage.NewLocal(s.cfg.StorageDir)
		if err != nil {
			slog.Error("failed to init local storage", "err", err)
			return nil
		}
		slog.Info("object storage backend", "type", "local", "dir", s.cfg.StorageDir)
		return store
	}
}

func (s *Server) routes() {
	// Liveness / readiness — used by Docker Compose healthchecks and CI.
	s.engine.GET("/healthz", s.handleHealthz)
	s.engine.GET("/readyz", s.handleReadyz)
	// Prometheus scrape endpoint (HTTP + Go/process metrics).
	s.engine.GET("/metrics", gin.WrapH(metrics.Handler()))

	// Versioned API surface. Module routers register under this group in
	// later PRs, e.g. auth.Register(api), dataset.Register(api), ...
	api := s.engine.Group("/api/v1")
	api.GET("/ping", func(c *gin.Context) {
		httpx.OK(c, gin.H{"pong": true, "env": s.cfg.Env})
	})

	// --- module wiring (modular monolith) ---
	// db may be nil in route-only tests; modules needing it are skipped then.
	if s.db != nil {
		tm := auth.NewTokenManager(s.cfg.JWTSecret, s.cfg.JWTAccessTTL, s.cfg.JWTRefreshTTL)
		var verifier auth.KYCVerifier = auth.ManualVerifier{}
		if s.cfg.KYCAutoApprove {
			verifier = auth.AutoApproveVerifier{}
		}
		authSvc := auth.NewService(auth.NewRepository(s.db), tm,
			auth.WithKYC(verifier, s.cfg.PIISecret),
			auth.WithDenylist(s.denylist()))
		lim := s.limiter() // shared rate limiter (auth credential routes + dataset preview)
		auth.Register(api, authSvc, tm, lim)

		authMW := auth.Middleware(tm)
		rec := audit.New(s.db)
		store := s.objectStorage() // shared by dataset (upload) and delivery (download)

		dsOpts := []dataset.Option{dataset.WithAsyncQuality(2, 128)}
		if store != nil {
			dsOpts = append(dsOpts, dataset.WithStorage(store))
		}
		if url := os.Getenv("QUALITY_SIDECAR_URL"); url != "" {
			dsOpts = append(dsOpts, dataset.WithAuthenticitySidecar(url, 15*time.Second))
		}
		dsSvc := dataset.NewService(dataset.NewRepository(s.db), authSvc, rec, dsOpts...)
		s.closers = append(s.closers, dsSvc.Close)
		dataset.Register(api, dsSvc, authMW, auth.RequireRole("ops", "admin"), lim)

		orderSvc := order.NewService(order.NewRepository(s.db), authSvc, datasetPurchaseAdapter{ds: dsSvc}, rec)
		order.Register(api, orderSvc, authMW, auth.RequireRole("ops", "admin"))

		// Payment + split-settlement provider selection.
		//  - stripe: REAL Stripe Connect (test mode = free). Separate charges &
		//    transfers = escrow-then-settle (docs §2.1).
		//  - mock: in-process sandbox (default; no real gateway).
		// WeChat/Alipay real integration still requires Spike-2 + 法务.
		var provider payment.PaymentProvider
		var split payment.SplitProvider
		if s.cfg.PaymentProvider == "stripe" && s.cfg.StripeSecretKey != "" {
			sp := payment.NewStripeProvider(s.cfg.StripeSecretKey, s.cfg.StripeWebhookSecret, s.cfg.StripeCurrency,
				stripePayoutStore{auth: authSvc})
			provider, split = sp, sp
			slog.Info("payment provider", "type", "stripe", "currency", s.cfg.StripeCurrency)
		} else {
			if s.cfg.PaymentProvider != "mock" {
				slog.Warn("payment provider unavailable; falling back to sandbox mock", "requested", s.cfg.PaymentProvider)
			}
			mock := payment.MockProvider{Secret: s.cfg.PaymentMockSecret}
			provider, split = mock, mock
		}
		paySvc := payment.NewService(payment.NewRepository(s.db), orderPaymentAdapter{o: orderSvc}, provider, split, rec)
		// H3: durable settlement — outbox + PG advisory lock + retry worker.
		paySvc.StartSettlementOutbox(payment.NewOutboxRepository(s.db), payment.NewPGLocker(s.db))
		s.closers = append(s.closers, paySvc.Close)
		payment.Register(api, paySvc, authMW, s.cfg.Env != "production")
		orderSvc.SetSettlementTrigger(paySvc) // confirm-delivery -> auto settle
		orderSvc.SetRefundTrigger(paySvc)     // dispute refund -> provider refund + reversal (H2)

		if store != nil {
			delSvc := delivery.NewService(delivery.NewRepository(s.db),
				orderDeliveryAdapter{o: orderSvc}, datasetDeliveryAdapter{ds: dsSvc},
				store, s.cfg.PIISecret, rec)
			delivery.Register(api, delSvc, authMW)
		}

		// Compute-to-Data (隐私计算 / 可用不可见). The buyer-invisible L1 sandbox:
		// buy a compute entitlement → run a whitelisted algorithm → get the
		// OUTPUT, never the raw data. The in-process worker runs only when object
		// storage is available (it stores outputs there); without it, jobs stay
		// queued for an out-of-process runner.
		//
		// Runner selection: COMPUTE_RUNNER=docker uses the hardened
		// `docker run --network none` sandbox (requires a Docker daemon + a built,
		// digest-pinned algorithm image); otherwise the in-process MockRunner
		// (default, docker-less dev/CI).
		var runner compute.Runner = compute.NewMockRunner()
		if os.Getenv("COMPUTE_RUNNER") == "docker" {
			runner = compute.NewDockerRunner(compute.DefaultDockerResources)
			slog.Info("compute runner", "kind", "docker")
		}
		var computeOpts []compute.Option
		if store != nil {
			computeOpts = append(computeOpts, compute.WithWorker(runner, store, dsSvc, 2, 64))
		}
		computeSvc := compute.NewService(compute.NewRepository(s.db), authSvc,
			computeDatasetAdapter{ds: dsSvc}, rec, computeOpts...)
		s.closers = append(s.closers, computeSvc.Close)
		// dev grant gated like payment's dev mark-paid (never in production).
		compute.Register(api, computeSvc, authMW, auth.RequireRole("ops", "admin"), s.cfg.Env != "production")
		// Refund→revoke (H2): when a dispute refund lands, revoke the buyer's
		// compute credits tied to that order.
		orderSvc.SetComputeRevoker(orderComputeAdapter{c: computeSvc})
	}
}

// computeDatasetAdapter bridges dataset.Service to compute.DatasetReader.
type computeDatasetAdapter struct{ ds *dataset.Service }

func (a computeDatasetAdapter) ForCompute(ctx context.Context, id string) (compute.DatasetInfo, error) {
	p, err := a.ds.ForPurchase(ctx, id)
	if err != nil {
		return compute.DatasetInfo{}, err
	}
	return compute.DatasetInfo{SellerID: p.SellerID, VersionID: p.VersionID, Published: p.Published}, nil
}

// orderComputeAdapter bridges compute.Service to order.ComputeRevoker so a
// refund revokes the buyer's compute entitlements without order importing compute.
type orderComputeAdapter struct{ c *compute.Service }

func (a orderComputeAdapter) RevokeEntitlementsForOrder(ctx context.Context, orderID string) (int, error) {
	return a.c.RevokeEntitlementsForOrder(ctx, orderID)
}

// orderDeliveryAdapter bridges order.Service to delivery.OrderGateway.
type orderDeliveryAdapter struct{ o *order.Service }

func (a orderDeliveryAdapter) GetSystem(ctx context.Context, id string) (delivery.OrderInfo, error) {
	o, err := a.o.GetSystem(ctx, id)
	if err != nil {
		return delivery.OrderInfo{}, err
	}
	return delivery.OrderInfo{ID: o.ID, BuyerID: o.BuyerID, Status: o.Status, DatasetID: o.DatasetID}, nil
}
func (a orderDeliveryAdapter) MarkDelivered(ctx context.Context, id string) error {
	_, err := a.o.MarkDelivered(ctx, id)
	return err
}

// datasetDeliveryAdapter bridges dataset.Service to delivery.DatasetReader.
type datasetDeliveryAdapter struct{ ds *dataset.Service }

func (a datasetDeliveryAdapter) CurrentObjectKey(ctx context.Context, datasetID string) (string, error) {
	return a.ds.CurrentObjectKey(ctx, datasetID)
}
