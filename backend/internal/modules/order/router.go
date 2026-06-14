package order

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts order routes. authMW protects buyer/seller actions; opsGate
// guards ops endpoints (transactions, dispute resolution). The service enforces
// party/ownership and the state machine. limiter throttles order creation and
// dispute filing (anti-flood, docs §6.8).
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	g := rg.Group("/orders")
	g.Use(authMW)
	g.POST("",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "order-create", Limit: 20, Window: time.Minute}),
		h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/confirm-delivery", h.confirmDelivery)
	g.POST("/:id/dispute",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "order-dispute", Limit: 10, Window: time.Minute}),
		h.dispute)
	g.POST("/:id/review", h.createReview)

	authed := rg.Group("")
	authed.Use(authMW)
	authed.GET("/sellers/me/earnings", h.earnings)

	// Public: dataset reviews.
	rg.GET("/datasets/:id/reviews", h.listReviews)

	// Ops.
	admin := rg.Group("/admin")
	admin.Use(authMW, opsGate)
	admin.GET("/transactions", h.adminTransactions)
	admin.POST("/orders/:id/resolve", h.resolveDispute)
}
