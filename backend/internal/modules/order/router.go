package order

import "github.com/gin-gonic/gin"

// Register mounts order routes. authMW protects buyer/seller actions; opsGate
// guards ops endpoints (transactions, dispute resolution). The service enforces
// party/ownership and the state machine.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc) {
	h := &handler{svc: svc}

	g := rg.Group("/orders")
	g.Use(authMW)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/confirm-delivery", h.confirmDelivery)
	g.POST("/:id/dispute", h.dispute)
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
	admin.GET("/reconciliation", h.adminReconciliation)
	admin.GET("/reconciliation/timeseries", h.adminReconciliationTimeseries)
	admin.POST("/orders/:id/resolve", h.resolveDispute)

	// Seller analytics.
	authed.GET("/sellers/me/earnings/timeseries", h.sellerEarningsTimeseries)
	authed.GET("/sellers/me/earnings/by-dataset", h.sellerEarningsByDataset)
}
