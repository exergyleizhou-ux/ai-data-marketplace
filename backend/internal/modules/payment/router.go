package payment

import "github.com/gin-gonic/gin"

// Register mounts payment routes. /payments/create requires auth (buyer);
// the webhook is public but signature-verified inside the service.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, devMode bool) {
	h := &handler{svc: svc}

	authed := rg.Group("/payments")
	authed.Use(authMW)
	authed.POST("/create", h.create)
	if devMode {
		// SANDBOX ONLY: simulate a paid callback so the UI can demo the loop
		// without a real gateway. Never mounted when APP_ENV=production.
		authed.POST("/dev/mark-paid", h.devMarkPaid)
	}

	// Provider callback — no JWT; authenticity via signature verification.
	rg.POST("/payments/webhook/:channel", h.webhook)

	// Ops: settlement outbox monitoring + manual retry.
	admin := rg.Group("/admin/settlement-outbox")
	admin.Use(authMW, opsGate)
	admin.GET("", h.adminListOutbox)
	admin.POST("/:orderId/retry", h.adminRetryOutbox)
}
