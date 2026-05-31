// Package server constructs the HTTP engine and wires module routers.
//
// Architectural rule (modular monolith): modules expose a router-registration
// function; the server composes them here. Modules MUST NOT reach into each
// other's packages or tables directly — only through exported interfaces.
package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/dataset"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	redispkg "github.com/lei/ai-data-marketplace/backend/internal/platform/redis"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type Server struct {
	cfg    *config.Config
	db     *pgxpool.Pool
	engine *gin.Engine
}

// New builds the server. db may be nil in tests that exercise only routes that
// don't touch the database.
func New(cfg *config.Config, db *pgxpool.Pool) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	// RequestID first so logger/recovery can correlate; Recovery wraps handlers.
	engine.Use(middleware.RequestID(), middleware.Logger(), middleware.Recovery())

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

// objectStorage builds the configured object-storage driver. Returns nil (and
// logs) on failure so upload endpoints degrade to "storage unavailable" rather
// than crashing the whole server.
func (s *Server) objectStorage() storage.Storage {
	switch s.cfg.StorageDriver {
	case "oss":
		slog.Warn("OSS storage driver is a stub; uploads will return not-implemented")
		return &storage.OSS{}
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
			auth.WithKYC(verifier, s.cfg.PIISecret))
		auth.Register(api, authSvc, tm, s.limiter())

		authMW := auth.Middleware(tm)
		rec := audit.New(s.db)

		dsOpts := []dataset.Option{}
		if store := s.objectStorage(); store != nil {
			dsOpts = append(dsOpts, dataset.WithStorage(store))
		}
		dsSvc := dataset.NewService(dataset.NewRepository(s.db), authSvc, rec, dsOpts...)
		dataset.Register(api, dsSvc, authMW, auth.RequireRole("ops", "admin"))
	}
}
