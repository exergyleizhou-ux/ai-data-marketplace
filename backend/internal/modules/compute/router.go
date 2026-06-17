package compute

import "github.com/gin-gonic/gin"

// Register mounts compute routes.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := NewHandler(svc)

	// Authenticated routes: algorithm registration / listing
	algo := rg.Group("")
	algo.Use(authMW)
	algo.POST("/compute/algorithms", h.RegisterAlgo)
	algo.GET("/compute/algorithms/current", h.ListCurrentAlgos)
	algo.GET("/compute/algorithms/mine", h.ListMyAlgos)
	algo.GET("/compute/algorithms/:id", h.GetAlgo)

	// Job submission & listing
	algo.POST("/compute/jobs", h.SubmitJob)
	algo.GET("/compute/jobs/mine", h.ListMyJobs)
	algo.GET("/compute/jobs/:id", h.GetJob)

	// Attestation verification
	algo.POST("/compute/verify", h.VerifyAttestation)
}
