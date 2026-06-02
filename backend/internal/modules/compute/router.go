package compute

import "github.com/gin-gonic/gin"

// Register mounts the compute (C2D) routes. authMW protects buyer/seller
// actions; opsGate guards the algorithm registry + job queue. devEnabled mounts
// the dev-only direct entitlement grant (no real payment gateway yet — real
// compute purchase via order+payment is a follow-up), mirroring payment's
// dev mark-paid gating.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, devEnabled bool) {
	h := &handler{svc: svc, devEnabled: devEnabled}

	// Public read: a dataset's offer (buyers see price / trust level).
	rg.GET("/datasets/:id/compute-offer", h.getOffer)

	// Seller: configure the offer (ownership enforced in the service).
	seller := rg.Group("")
	seller.Use(authMW)
	seller.PUT("/datasets/:id/compute-offer", h.putOffer)

	// Buyer.
	buyer := rg.Group("")
	buyer.Use(authMW)
	buyer.GET("/compute/algorithms", h.listAlgorithms)
	buyer.POST("/compute/jobs", h.submitJob)
	buyer.GET("/compute/jobs/:id", h.getJob)
	buyer.GET("/compute/jobs/:id/output", h.downloadOutput)
	buyer.POST("/compute/jobs/:id/cancel", h.cancelJob)
	buyer.GET("/users/me/compute/jobs", h.listMyJobs)
	buyer.GET("/users/me/compute/entitlements", h.listMyEntitlements)
	// Real purchase: create a compute order, then pay it via the payment flow.
	buyer.POST("/datasets/:id/compute/order", h.createComputeOrder)

	if devEnabled {
		// Dev-only: grant a compute entitlement without a real gateway so the
		// loop is demoable. Never mounted when APP_ENV=production.
		buyer.POST("/datasets/:id/compute/purchase", h.purchase)
	}

	// Ops: algorithm registry review + job queue.
	admin := rg.Group("/admin/compute")
	admin.Use(authMW, opsGate)
	admin.GET("/algorithms", h.adminListAlgorithms)
	admin.POST("/algorithms", h.registerAlgorithm)
	admin.POST("/algorithms/:id/review", h.reviewAlgorithm)
	admin.GET("/jobs", h.adminListJobs)
	admin.POST("/jobs/:id/release", h.opsReleaseJob) // release an output_reviewing job
	admin.POST("/jobs/:id/reject", h.opsRejectJob)   // reject it (output withheld, credit refunded)
}
