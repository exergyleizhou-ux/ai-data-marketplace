// Package server constructs the HTTP engine and wires module routers.
//
// Architectural rule (modular monolith): modules expose a router-registration
// function; the server composes them here. Modules MUST NOT reach into each
// other's packages or tables directly — only through exported interfaces.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/config"
)

type Server struct {
	cfg    *config.Config
	engine *gin.Engine
}

func New(cfg *config.Config) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(gin.Recovery())

	s := &Server{cfg: cfg, engine: engine}
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
	_ = s.engine.Group("/api/v1")
}
