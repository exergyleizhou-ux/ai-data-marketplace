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
