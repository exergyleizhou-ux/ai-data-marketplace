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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/anomaly"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auditlog"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/compliance"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/compute"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/dataset"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/delivery"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/docs"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/notification"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/order"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/payment"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/qa"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/search"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/verify"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/watchlist"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/withdrawal"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/metrics"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	redispkg "github.com/lei/ai-data-marketplace/backend/internal/platform/redis"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
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

// startBackgroundCleaners launches periodic maintenance goroutines (token
// expiry cleanup, consumed code purging). Registered in s.closers so Close()
// stops them gracefully.
func (s *Server) startBackgroundCleaners() {
	ctx, cancel := context.WithCancel(context.Background())
	s.closers = append(s.closers, cancel)

	// Password-reset token cleanup: every hour, delete tokens expired > 7 days.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.db.Exec(ctx,
					`DELETE FROM password_reset_tokens WHERE expires_at < now() - INTERVAL '7 days'`); err != nil {
					slog.Warn("password-reset token cleanup failed", "err", err)
				}
			}
		}
	}()

	// TOTP recovery codes: purge codes consumed > 30 days ago (daily sweep).
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.db.Exec(ctx,
					`DELETE FROM totp_recovery_codes WHERE used_at IS NOT NULL AND used_at < now() - INTERVAL '30 days'`); err != nil {
					slog.Warn("totp recovery code cleanup failed", "err", err)
				}
			}
		}
	}()

	slog.Info("background cleaners started", "tasks", 2)
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
		// otelgin starts a span per request (no-op unless tracing.Init enabled
		// a provider); it must run before TraceID so trace_id == span trace ID.
		otelgin.Middleware("marketplace-backend"),
		middleware.RequestID(),
		middleware.TraceID(),
		middleware.RemoveServerHeader(),
		middleware.SecurityHeaders(cfg.Env),
		middleware.CacheControl(),
		metrics.Middleware(),
		middleware.Logger(),
		middleware.Recovery(),
	)

	s := &Server{cfg: cfg, db: db, engine: engine}
	if db != nil {
		s.startBackgroundCleaners()
	}
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

		// Search module: thin public search endpoint backed by the dataset index.
		search.Register(api, datasetSearchAdapter{ds: dsSvc})

		orderSvc := order.NewService(order.NewRepository(s.db), authSvc, datasetPurchaseAdapter{ds: dsSvc}, rec)
		order.Register(api, orderSvc, authMW, auth.RequireRole("ops", "admin"), lim)

		// Notification module: user-facing event feed (order paid/settled/disputed,
		// quality done, compute released). Wired as a Notifier into order + dataset.
		notifyRepo := notification.NewRepository(s.db)
		prefsRepo := notification.NewPreferencesRepository(s.db)
		emailLogRepo := notification.NewEmailLogRepository(s.db)
		var emailSender notification.EmailSender
		if host := os.Getenv("SMTP_HOST"); host != "" {
			emailSender = notification.NewSMTPSender(host, envDefault("SMTP_PORT", "587"),
				os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASS"),
				envDefault("SMTP_FROM_ADDR", "noreply@verdant-oasis.dev"),
				envDefault("SMTP_FROM_NAME", "绿洲 Verdant Oasis"))
			slog.Info("notification email channel enabled", "smtp", host)
		}
		notifySvc := notification.NewServiceWithChannels(notifyRepo, prefsRepo, emailSender, emailLogRepo,
			notificationUserLookup{pool: s.db})
		notification.Register(api, notifySvc, authMW)
		orderSvc.SetNotifier(notifySvc)     // order events → buyer/seller notifications
		dsSvc.SetQualityNotifier(notifySvc) // quality done → seller notification
		authSvc.SetNotifier(notifySvc)      // password reset email
		authSvc.SetAppBaseURL(s.cfg.AppBaseURL)

		// Bundle download: orderSvc needs object storage + dataset key resolver.
		if store != nil {
			orderSvc.SetBundleSource(datasetBundleAdapter{ds: dsSvc}, store)
		}

		// Certificate verification: public lookup endpoint + cert registration.
		verifyRepo := verify.NewRepository(s.db)
		verify.Register(api, verifyRepo)

		// Audit-log viewer: ops-only, read-only over audit_logs.
		auditSvc := auditlog.NewService(auditlog.NewRepository(s.db))
		auditlog.Register(api, auditSvc, authMW, auth.RequireRole("ops", "admin"))
		dsSvc.SetCertRegistrar(verifyRepo) // dataset certs → registered for public lookup

		// Watchlist: dataset watching + new-version notification.
		watchRepo := watchlist.NewRepository(s.db)
		watchSvc := watchlist.NewService(watchRepo, notifySvc, watchlistDatasetAdapter{ds: dsSvc})
		watchlist.Register(api, watchSvc, authMW)
		dsSvc.SetWatchersNotifier(watchSvc)

		// Dataset Q&A: buyer asks + seller answers (PR-O).
		qaRepo := qa.NewRepository(s.db)
		qaSvc := qa.NewService(qaRepo, qaDatasetAdapter{ds: dsSvc}, notifySvc)
		qa.Register(api, qaSvc, authMW, lim)

		// Withdrawal: seller requests + ops approves (book-keeping only, P module).
		withdrawRepo := withdrawal.NewRepository(s.db)
		withdrawSvc := withdrawal.NewService(withdrawRepo, withdrawEarningsAdapter{order: orderSvc}, notifySvc)
		withdrawal.Register(api, withdrawSvc, authMW, auth.RequireRole("ops", "admin"), lim)

		// Anomaly scanner: periodic audit pattern detection (PR-Q).
		anomalyRepo := anomaly.NewRepository(s.db)
		var anomalyAlerter anomaly.Alerter = anomaly.NopAlerter{}
		if whURL := os.Getenv("ANOMALY_WEBHOOK_URL"); whURL != "" {
			kinds := strings.Split(os.Getenv("ANOMALY_WEBHOOK_KINDS"), ",")
			if len(kinds) == 1 && kinds[0] == "" {
				kinds = []string{"high_risk_action", "repeated_failure"}
			}
			anomalyAlerter = anomaly.NewWebhookAlerter(whURL, kinds)
			slog.Info("anomaly webhook alerting enabled", "url", whURL[:minInt(30, len(whURL))])
		}
		anomalySvc := anomaly.NewService(anomalyRepo, s.db, anomalyAlerter)
		anomalySvc.StartScanner(context.Background())
		anomaly.Register(api, anomalySvc, authMW, auth.RequireRole("ops", "admin"))

		// PIPL Compliance: data export + account deletion (PR-S).
		compExportSvc := compliance.NewExportService(compliance.NewExportRepository(s.db),
			complianceSourceAdapter{pool: s.db}, notifySvc, store)
		compExportSvc.StartScanner(context.Background())
		compDeletionSvc := compliance.NewDeletionService(compliance.NewDeletionRepository(s.db), notifySvc)
		compliance.Register(api, compExportSvc, compDeletionSvc, authMW, auth.RequireRole("ops", "admin"))
		// compute certs: registered in compute module via the same interface
		// (wired below after computeSvc is constructed)

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
		payment.Register(api, paySvc, authMW, auth.RequireRole("ops", "admin"), lim, s.cfg.Env != "production")
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
		// Runner selection (COMPUTE_RUNNER):
		//   ""/mock  in-process MockRunner (default, docker-less dev/CI)
		//   docker   hardened `docker run --network none` sandbox (L1)
		//   tee      docker sandbox wrapped with remote attestation (L2, P3)
		var runner compute.Runner = compute.NewMockRunner()
		var computeOpts []compute.Option
		switch os.Getenv("COMPUTE_RUNNER") {
		case "docker":
			res := compute.DefaultDockerResources
			res.Runtime = os.Getenv("COMPUTE_DOCKER_RUNTIME") // "" runc | "runsc" gVisor | "kata" (P2, §7.2)
			runner = compute.NewDockerRunner(res)
			slog.Info("compute runner", "kind", "docker", "runtime", res.Runtime)
		case "tee":
			res := compute.DefaultDockerResources
			res.Runtime = os.Getenv("COMPUTE_DOCKER_RUNTIME")
			// Attester selection (TEE_ATTESTER): "tdx" reads a real hardware quote
			// from /dev/tdx_guest on a TDX node (fails closed off-hardware); default
			// is the non-hardware MockAttester (HMAC-bound, tamper-evident) for dev.
			var att compute.Attester = compute.NewMockAttester()
			attKind := "mock"
			if os.Getenv("TEE_ATTESTER") == "tdx" {
				att = compute.NewTDXAttester()
				attKind = "tdx"
			}
			// Attestation-based key release (KBS): gates data access on a verified
			// attestation before the algorithm runs (design P3 §4 / Direction B). With
			// KBS_URL set, release goes through a real KBS over HTTP (the KBS verifies
			// the attestation against the hardware root + measurement policy and is the
			// trust boundary that keeps data "invisible even to the platform"). Without
			// it, the dev mockKBS verifies the MockAttester report (no real privacy).
			kbsKind := "mock"
			var kbs compute.KeyBroker = compute.NewMockKBS(att)
			if u := os.Getenv("KBS_URL"); u != "" {
				kbs = compute.NewRemoteKBS(u)
				kbsKind = "remote"
			}
			runner = compute.NewTEERunnerWithKBS(compute.NewDockerRunner(res), att, kbs)
			computeOpts = append(computeOpts, compute.WithAttester(att))
			slog.Info("compute runner", "kind", "tee", "runtime", res.Runtime, "attester", attKind, "kbs", kbsKind)
		}
		if store != nil {
			computeOpts = append(computeOpts, compute.WithWorker(runner, store, dsSvc, 2, 64))
		}
		computeSvc := compute.NewService(compute.NewRepository(s.db), authSvc,
			computeDatasetAdapter{ds: dsSvc}, rec, computeOpts...)
		s.closers = append(s.closers, computeSvc.Close)
		// dev grant gated like payment's dev mark-paid (never in production).
		compute.Register(api, computeSvc, authMW, auth.RequireRole("ops", "admin"), lim, s.cfg.Env != "production")
		// Refund→revoke (H2): when a dispute refund lands, revoke the buyer's
		// compute credits tied to that order.
		orderSvc.SetComputeRevoker(orderComputeAdapter{c: computeSvc})
		// Real purchase: compute creates orders via order; paying an order grants
		// the entitlement via compute. Two late-bound hooks (neither imports the
		// other).
		computeSvc.SetOrderCreator(computeOrderAdapter{o: orderSvc})
		orderSvc.SetComputeGranter(orderComputeGranterAdapter{c: computeSvc})
	}

	// API documentation (public, no auth).
	docs.Register(s.engine)
}

// computeOrderAdapter bridges order.Service to compute.OrderCreator, translating
// order's sentinels to compute's so the compute handler renders the right error.
type computeOrderAdapter struct{ o *order.Service }

func (a computeOrderAdapter) CreateComputeOrder(ctx context.Context, buyerID, sellerID, datasetID string, amountCents int64) (string, error) {
	o, err := a.o.CreateCompute(ctx, buyerID, sellerID, datasetID, amountCents)
	switch {
	case errors.Is(err, order.ErrNotVerified):
		return "", compute.ErrNotVerified
	case errors.Is(err, order.ErrSelfPurchase):
		return "", compute.ErrSelfPurchase
	case errors.Is(err, order.ErrDuplicateOrder):
		return "", compute.ErrPurchasePending
	case err != nil:
		return "", err
	}
	return o.ID, nil
}

// orderComputeGranterAdapter bridges compute.Service to order.ComputeGranter so
// paying a compute order grants its entitlement.
type orderComputeGranterAdapter struct{ c *compute.Service }

func (a orderComputeGranterAdapter) GrantForOrder(ctx context.Context, orderID, datasetID, buyerID string) error {
	return a.c.GrantForOrder(ctx, orderID, datasetID, buyerID)
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

// datasetSearchAdapter bridges dataset.Service to search.DatasetSearcher
// so the search module can read published datasets without importing dataset.
type datasetSearchAdapter struct{ ds *dataset.Service }

func (a datasetSearchAdapter) SearchPublished(ctx context.Context, q search.SearchQuery) ([]search.SearchResult, error) {
	items, err := a.ds.SearchPublished(ctx, dataset.ListFilter{
		Keyword:  q.Keyword,
		DataType: q.DataType,
		Domain:   q.Domain,
		Sort:     q.Sort,
		Limit:    q.Limit,
		Offset:   q.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]search.SearchResult, len(items))
	for i, d := range items {
		pc := int64(0)
		if d.FinalPriceCents != nil {
			pc = *d.FinalPriceCents
		} else if d.SuggestedPriceCents != nil {
			pc = *d.SuggestedPriceCents
		}
		out[i] = search.SearchResult{
			ID:               d.ID,
			Title:            d.Title,
			DataType:         d.DataType,
			Domain:           d.Domain,
			PriceCents:       pc,
			Status:           d.Status,
			AuthenticityBand: d.AuthenticityBand,
		}
	}
	return out, nil
}

// datasetBundleAdapter bridges dataset.Service to order.BundleSource.
type datasetBundleAdapter struct{ ds *dataset.Service }

func (a datasetBundleAdapter) CurrentObjectKey(ctx context.Context, datasetID string) (string, error) {
	return a.ds.CurrentObjectKey(ctx, datasetID)
}
func (a datasetBundleAdapter) SuggestFilename(ctx context.Context, datasetID string) (string, error) {
	d, err := a.ds.Get(ctx, datasetID)
	if err != nil {
		return "", err
	}
	title := d.Title
	if title == "" {
		title = datasetID
	}
	ext := ".bin"
	switch d.DataType {
	case "csv", "text/csv":
		ext = ".csv"
	case "json", "application/json":
		ext = ".json"
	case "parquet":
		ext = ".parquet"
	default:
		ext = ".txt"
	}
	return sanitiseFilename(title) + ext, nil
}

func sanitiseFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			out = append(out, c)
		} else if c == ' ' {
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "dataset"
	}
	return string(out)
}

// watchlistDatasetAdapter bridges dataset.Service to watchlist.DatasetReader.
type watchlistDatasetAdapter struct{ ds *dataset.Service }

func (a watchlistDatasetAdapter) StatusOf(ctx context.Context, datasetID string) (string, error) {
	d, err := a.ds.Get(ctx, datasetID)
	if err != nil {
		return "", err
	}
	return d.Status, nil
}

// qaDatasetAdapter bridges dataset.Service to qa.DatasetReader.
type qaDatasetAdapter struct{ ds *dataset.Service }

func (a qaDatasetAdapter) SellerOf(ctx context.Context, datasetID string) (string, string, error) {
	d, err := a.ds.Get(ctx, datasetID)
	if err != nil {
		return "", "", err
	}
	return d.SellerID, d.Status, nil
}

// withdrawEarningsAdapter bridges order.Service to withdrawal.EarningsReader.
type withdrawEarningsAdapter struct{ order *order.Service }

func (a withdrawEarningsAdapter) SettledCentsOf(ctx context.Context, sellerID string) (int64, error) {
	e, err := a.order.Earnings(ctx, sellerID)
	if err != nil {
		return 0, err
	}
	return e.SettledCents, nil
}

// complianceSourceAdapter bridges various modules to compliance.Source.
type complianceSourceAdapter struct{ pool *pgxpool.Pool }

func (a complianceSourceAdapter) UserRow(ctx context.Context, userID string) (map[string]any, error) {
	var account, role, kycStatus, createdAt string
	if err := a.pool.QueryRow(ctx,
		`SELECT account, role, kyc_status, created_at::text FROM users WHERE id=$1`, userID).
		Scan(&account, &role, &kycStatus, &createdAt); err != nil {
		return nil, err
	}
	return map[string]any{"id": userID, "account": account, "role": role, "kyc_status": kycStatus, "created_at": createdAt}, nil
}
func (a complianceSourceAdapter) Orders(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, status, amount_cents, product_type, created_at::text FROM orders WHERE buyer_id=$1 OR seller_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, status, productType, createdAt string
		var cents int64
		rows.Scan(&id, &status, &cents, &productType, &createdAt)
		out = append(out, map[string]any{"id": id, "status": status, "amount_cents": cents, "product_type": productType, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Datasets(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, title, status, data_type, created_at::text FROM datasets WHERE seller_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, title, status, dataType, createdAt string
		rows.Scan(&id, &title, &status, &dataType, &createdAt)
		out = append(out, map[string]any{"id": id, "title": title, "status": status, "data_type": dataType, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Notifications(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, kind, title, COALESCE(body,''), is_read, created_at::text FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, kind, title, body, createdAt string
		var isRead bool
		rows.Scan(&id, &kind, &title, &body, &isRead, &createdAt)
		out = append(out, map[string]any{"id": id, "kind": kind, "title": title, "body": body, "is_read": isRead, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Watches(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT dataset_id::text, last_notified_version_id::text, created_at::text FROM dataset_watches WHERE user_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var dsID, ver, createdAt string
		rows.Scan(&dsID, &ver, &createdAt)
		out = append(out, map[string]any{"dataset_id": dsID, "last_notified_version_id": ver, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Questions(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, dataset_id::text, body, status, created_at::text FROM dataset_questions WHERE asker_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, dsID, body, status, createdAt string
		rows.Scan(&id, &dsID, &body, &status, &createdAt)
		out = append(out, map[string]any{"id": id, "dataset_id": dsID, "body": body, "status": status, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Answers(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, question_id::text, body, created_at::text FROM dataset_answers WHERE answerer_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, qID, body, createdAt string
		rows.Scan(&id, &qID, &body, &createdAt)
		out = append(out, map[string]any{"id": id, "question_id": qID, "body": body, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Reviews(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, dataset_id::text, score, COALESCE(comment,''), created_at::text FROM reviews WHERE buyer_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, dsID, comment, createdAt string
		var score int
		rows.Scan(&id, &dsID, &score, &comment, &createdAt)
		out = append(out, map[string]any{"id": id, "dataset_id": dsID, "score": score, "comment": comment, "created_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) Withdrawals(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, amount_cents, channel, status, requested_at::text FROM withdrawal_requests WHERE seller_id=$1 ORDER BY requested_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, channel, status, createdAt string
		var cents int64
		rows.Scan(&id, &cents, &channel, &status, &createdAt)
		out = append(out, map[string]any{"id": id, "amount_cents": cents, "channel": channel, "status": status, "requested_at": createdAt})
	}
	return out, nil
}
func (a complianceSourceAdapter) ComputeJobs(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, dataset_id::text, status, output_kind, created_at::text FROM compute_jobs WHERE buyer_id=$1 ORDER BY created_at DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, dsID, status, kind, createdAt string
		rows.Scan(&id, &dsID, &status, &kind, &createdAt)
		out = append(out, map[string]any{"id": id, "dataset_id": dsID, "status": status, "output_kind": kind, "created_at": createdAt})
	}
	return out, nil
}

// notificationUserLookup bridges users table to notification.UserLookup.
type notificationUserLookup struct{ pool *pgxpool.Pool }

func (a notificationUserLookup) EmailOf(ctx context.Context, userID string) (string, error) {
	var account string
	if err := a.pool.QueryRow(ctx, `SELECT account FROM users WHERE id=$1`, userID).Scan(&account); err != nil {
		return "", err
	}
	return account, nil
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
