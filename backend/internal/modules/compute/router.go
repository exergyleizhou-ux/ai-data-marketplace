package compute

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts the compute (C2D) routes. authMW protects buyer/seller
// actions; opsGate guards the algorithm registry + job queue. devEnabled mounts
// the dev-only direct entitlement grant (no real payment gateway yet — real
// compute purchase via order+payment is a follow-up), mirroring payment's
// dev mark-paid gating.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter, devEnabled bool) {
	h := &handler{svc: svc, devEnabled: devEnabled}

	// Public read: a dataset's offer (buyers see price / trust level).
	rg.GET("/datasets/:id/compute-offer", h.getOffer)
	// Public batch: compute-to-data discovery signals for the catalog (which
	// datasets support verifiable sandbox compute, trust level, usage count).
	rg.GET("/compute/offers/signals", h.offerSignals)

	// Seller: configure the offer (ownership enforced in the service).
	seller := rg.Group("")
	seller.Use(authMW)
	seller.PUT("/datasets/:id/compute-offer", h.putOffer)

	// Buyer.
	buyer := rg.Group("")
	buyer.Use(authMW)
	buyer.GET("/compute/algorithms", h.listAlgorithms)
	buyer.POST("/compute/jobs",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "compute_job_submit", Limit: 20, Window: time.Minute}),
		h.submitJob)
	buyer.GET("/compute/jobs/:id", h.getJob)
	buyer.GET("/compute/jobs/:id/output", h.downloadOutput)
	buyer.GET("/compute/jobs/:id/attestation", h.jobAttestation) // L2 remote-attestation (P3)
	buyer.GET("/compute/jobs/:id/certificate", h.jobCertificate) // 计算结果存证 (provenance certificate)
	buyer.POST("/compute/jobs/:id/cancel",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "compute_job_cancel", Limit: 20, Window: time.Minute}),
		h.cancelJob)
	// Federated learning (P4-a): one job across N datasets; sub-jobs run in each
	// dataset's sandbox, only the aggregated joint model is buyer-visible.
	buyer.POST("/compute/federated-jobs",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "compute_federated_submit", Limit: 10, Window: time.Minute}),
		h.submitFederated)
	buyer.GET("/compute/federated-jobs/:id", h.getFederated)
	buyer.GET("/compute/federated-jobs/:id/output", h.federatedOutput)
	buyer.GET("/compute/federated-jobs/:id/certificate", h.federatedCertificate) // 联合结果存证
	buyer.GET("/users/me/compute/federated-jobs", h.listMyFederated)
	buyer.GET("/users/me/compute/jobs", h.listMyJobs)
	buyer.GET("/users/me/compute/entitlements", h.listMyEntitlements)
	// Custom-algorithm submission: forced pending + untrusted (never runnable
	// until ops approve), rate-limited to deter registry spam.
	buyer.POST("/compute/algorithm-requests",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "algo_request", Limit: 5, Window: time.Minute}),
		h.requestAlgorithm)
	buyer.GET("/users/me/compute/algorithm-requests", h.listMyAlgorithmRequests)
	// Real purchase: create a compute order, then pay it via the payment flow.
	buyer.POST("/datasets/:id/compute/order",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "compute_order", Limit: 10, Window: time.Minute}),
		h.createComputeOrder)

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
