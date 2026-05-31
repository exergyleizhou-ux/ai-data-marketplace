package dataset

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts dataset routes. authMW protects seller mutations; opsGate
// guards the admin review endpoints; limiter throttles the preview endpoint.
// The server supplies these (built from auth/platform) so this package stays
// decoupled.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	// Public browse/search + detail.
	rg.GET("/datasets", h.list)
	rg.GET("/datasets/:id", h.get)

	// Sample preview: login required + rate-limited (anti-scrape, docs §2.3/§6.4).
	preview := rg.Group("")
	preview.Use(authMW, middleware.RateLimit(limiter, middleware.RateLimitConfig{
		Name: "preview", Limit: 30, Window: time.Minute,
	}))
	preview.GET("/datasets/:id/preview", h.preview)

	// Authenticated routes (service enforces KYC + ownership).
	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/datasets", h.create)
	authed.PUT("/datasets/:id", h.update)
	authed.POST("/datasets/:id/source-declaration/sign", h.signSource)
	authed.GET("/users/me/datasets", h.listMine) // separate path to avoid /datasets/:id conflict

	// Chunked upload (PR-08). part/complete/status resolve the dataset from the
	// upload id, so the :id segment is only meaningful for init.
	authed.POST("/datasets/:id/upload/init", h.initUpload)
	authed.PUT("/datasets/:id/upload/part", h.uploadPart)
	authed.POST("/datasets/:id/upload/complete", h.completeUpload)
	authed.GET("/datasets/:id/upload/status", h.uploadStatus)

	// Ops review / takedown.
	admin := rg.Group("/admin/datasets")
	admin.Use(authMW, opsGate)
	admin.POST("/:id/review", h.review)
	admin.POST("/:id/delist", h.delist)
}
