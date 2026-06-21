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
	pub.POST("/refresh",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "refresh", Limit: 30, Window: time.Minute}),
		h.refresh)
	pub.POST("/logout",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "logout", Limit: 30, Window: time.Minute}),
		h.logout)

	// Protected routes require a valid access token.
	authed := rg.Group("")
	authed.Use(Middleware(tm))
	authed.GET("/users/me", h.me)
	authed.PUT("/users/me", h.updateProfile)
	authed.POST("/users/me/kyc", h.submitKYC)
	authed.GET("/users/me/kyc", h.getKYC)
	authed.GET("/users/me/agreements", h.listAgreements)
	authed.POST("/users/me/agreements", h.recordAgreements)

	// 2FA TOTP (mutations rate-limited — credential-surface abuse / TOTP brute-force).
	authed.POST("/auth/2fa/enroll",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "2fa_enroll", Limit: 5, Window: time.Minute}),
		h.enroll2FA)
	authed.POST("/auth/2fa/verify-enrollment",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "2fa_verify_enroll", Limit: 10, Window: time.Minute}),
		h.verify2FAEnrollment)
	authed.POST("/auth/2fa/disable",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "2fa_disable", Limit: 5, Window: time.Minute}),
		h.disable2FA)
	authed.GET("/auth/2fa/recovery-status", h.recoveryCodeStatus)

	// Public: 2FA verify (post-login + challenge), rate-limited.
	pub.POST("/2fa/verify",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "2fa_verify", Limit: 15, Window: time.Minute}),
		h.verify2FAChallenge)

	// Public: password reset (anti-enumeration on request; rate-limited).
	pub.POST("/password-reset/request",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "password_reset_request", Limit: 3, Window: time.Minute}),
		h.requestPasswordReset)
	pub.POST("/password-reset/complete",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "password_reset_complete", Limit: 5, Window: time.Minute}),
		h.completePasswordReset)

	// Ops-only review of KYC submissions.
	admin := rg.Group("/admin")
	admin.Use(Middleware(tm), RequireRole(roleOps, roleAdmin))
	admin.GET("/kyc/pending", h.adminListKYC)
	admin.POST("/kyc/review", h.adminReviewKYC)
	// Lawful retrieval of an applicant's raw ID number (decrypt + audit log).
	admin.GET("/kyc/:id/id-no", h.revealIDNo)
}
