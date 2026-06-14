package payment

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts payment routes. /payments/create requires auth (buyer);
// the webhook is public but signature-verified inside the service. limiter
// throttles payment creation (financial-operation protection, docs §6.8).
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc, limiter ratelimit.Limiter, devMode bool) {
	h := &handler{svc: svc}

	authed := rg.Group("/payments")
	authed.Use(authMW)
	authed.POST("/create",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "payment-create", Limit: 10, Window: time.Minute}),
		h.create)
	if devMode {
		// SANDBOX ONLY: simulate a paid callback so the UI can demo the loop
		// without a real gateway. Never mounted when APP_ENV=production.
		authed.POST("/dev/mark-paid", h.devMarkPaid)
	}

	// Provider callback — no JWT; authenticity via signature verification.
	rg.POST("/payments/webhook/:channel", h.webhook)
}
