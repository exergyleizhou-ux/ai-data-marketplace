package workbench

import (
	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	"time"
)

func Register(rg *gin.RouterGroup, s *Service, tm *auth.TokenManager, l ratelimit.Limiter, tokenEnabled bool) {
	h := handler{s: s, tokenEnabled: tokenEnabled}
	g := rg.Group("/workbench")
	g.Use(auth.Middleware(tm))
	g.POST("/token", middleware.RateLimit(l, middleware.RateLimitConfig{Name: "workbench_token", Limit: 30, Window: time.Minute}), h.token)
	g.GET("/runs", h.runs)
	g.GET("/runs/:id", h.run)
	g.GET("/runs/:id/events", h.events)
	g.GET("/runs/:id/approvals", h.approvals)
	g.POST("/approvals/:approvalID/decision", h.decide)
	g.GET("/runs/:id/artifacts", h.artifacts)
	g.GET("/artifacts/:artifactID/download", h.downloadArtifact)
}

// RegisterRuntimeIngest exposes the explicit Lumen-to-Oasis persistence contract.
// It is deliberately separate from browser authentication and always fails
// closed when its dedicated machine credential is absent.
func RegisterRuntimeIngest(rg *gin.RouterGroup, s *Service, secret string) {
	h := runtimeHandler{s: s}
	g := rg.Group("/workbench/runtime")
	g.Use(runtimeAuth(secret))
	g.PUT("/runs/:id", h.createRun)
	g.POST("/runs/:id/state", h.state)
	g.POST("/runs/:id/events", h.event)
	g.POST("/runs/:id/usage", h.usage)
	g.POST("/runs/:id/approvals", h.approval)
	g.POST("/approvals/:approvalID/consume", h.consume)
	g.POST("/approvals/:approvalID/execution", h.complete)
	g.POST("/runs/:id/artifacts", h.artifact)
}
