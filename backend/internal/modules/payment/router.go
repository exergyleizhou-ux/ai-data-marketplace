package payment

import "github.com/gin-gonic/gin"

// Register mounts payment routes. /payments/create requires auth (buyer);
// the webhook is public but signature-verified inside the service.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}

	authed := rg.Group("/payments")
	authed.Use(authMW)
	authed.POST("/create", h.create)

	// Provider callback — no JWT; authenticity via signature verification.
	rg.POST("/payments/webhook/:channel", h.webhook)
}
