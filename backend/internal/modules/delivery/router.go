package delivery

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// Register mounts delivery routes. The download-request requires auth (buyer);
// the file endpoint is public — the one-time token is the capability. The public
// /files route is rate limited so the token space can't be brute-forced /
// enumerated unthrottled.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/orders/:id/download",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "download_request", Limit: 30, Window: time.Minute}),
		h.requestDownload)

	rg.GET("/files/:token",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "file_download", Limit: 60, Window: time.Minute}),
		h.download)
}
