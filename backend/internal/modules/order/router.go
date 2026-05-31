package order

import "github.com/gin-gonic/gin"

// Register mounts order routes. All require authentication (server-injected
// authMW); the service enforces party/ownership and the state machine.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}
	g := rg.Group("/orders")
	g.Use(authMW)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/confirm-delivery", h.confirmDelivery)
	g.POST("/:id/dispute", h.dispute)
}
