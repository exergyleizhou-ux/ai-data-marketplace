package delivery

import "github.com/gin-gonic/gin"

// Register mounts delivery routes. The download-request requires auth (buyer);
// the file endpoint is public — the one-time token is the capability.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}

	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/orders/:id/download", h.requestDownload)

	rg.GET("/files/:token", h.download)
}
