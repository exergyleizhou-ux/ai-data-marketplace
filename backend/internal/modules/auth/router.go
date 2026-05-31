package auth

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts the auth routes under the given /api/v1 group. The server
// composes modules by calling each module's Register — this is the only entry
// point the server knows about (modular-monolith boundary).
//
// limiter rate-limits the credential endpoints (per client IP) to blunt
// brute-force and abuse (docs §6.8).
func Register(rg *gin.RouterGroup, svc *Service, tm *TokenManager, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	pub := rg.Group("/auth")
	pub.POST("/register",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "register", Limit: 5, Window: time.Minute}),
		h.register)
	pub.POST("/login",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "login", Limit: 10, Window: time.Minute}),
		h.login)
	pub.POST("/refresh", h.refresh)

	// Protected routes require a valid access token.
	authed := rg.Group("")
	authed.Use(Middleware(tm))
	authed.GET("/users/me", h.me)
	authed.PUT("/users/me", h.updateProfile)
	authed.POST("/users/me/kyc", h.submitKYC)
	authed.GET("/users/me/kyc", h.getKYC)

	// Ops-only review of KYC submissions.
	admin := rg.Group("/admin")
	admin.Use(Middleware(tm), RequireRole(roleOps, roleAdmin))
	admin.POST("/kyc/review", h.adminReviewKYC)
}
