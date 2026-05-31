package dataset

import "github.com/gin-gonic/gin"

// Register mounts dataset routes. authMW protects seller-only mutations; detail
// reads are public so buyers can browse. The server supplies authMW (built from
// the auth module) so this package stays decoupled from auth.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}

	// Public read.
	rg.GET("/datasets/:id", h.get)

	// Authenticated routes (service enforces KYC + ownership).
	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/datasets", h.create)
	authed.PUT("/datasets/:id", h.update)
	authed.POST("/datasets/:id/source-declaration/sign", h.signSource)
	authed.GET("/users/me/datasets", h.listMine) // separate path to avoid /datasets/:id conflict

	// Chunked upload (PR-08). part/complete/status resolve the dataset from the
	// upload id, so the :id segment is only meaningful for init.
	authed.POST("/datasets/:id/upload/init", h.initUpload)
	authed.PUT("/datasets/:id/upload/part", h.uploadPart)
	authed.POST("/datasets/:id/upload/complete", h.completeUpload)
	authed.GET("/datasets/:id/upload/status", h.uploadStatus)
}
