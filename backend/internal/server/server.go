// Package server constructs the HTTP engine and wires module routers.
//
// Architectural rule (modular monolith): modules expose a router-registration
// function; the server composes them here. Modules MUST NOT reach into each
// other's packages or tables directly — only through exported interfaces.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
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
		authSvc := auth.NewService(auth.NewRepository(s.db), tm)
		auth.Register(api, authSvc, tm)
	}
}
