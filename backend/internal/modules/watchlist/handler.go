package watchlist

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct{ svc *Service }

// Register mounts watchlist routes. All require auth (self-scoped). The write
// routes are rate limited to bound watch-row write amplification, matching every
// other write module.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	u := rg.Group("")
	u.Use(authMW, middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "watch", Limit: 60, Window: time.Minute}))
	u.POST("/datasets/:id/watch", h.add)
	u.DELETE("/datasets/:id/watch", h.remove)
	u.GET("/users/me/watched", h.listMy)
}

func (h *handler) add(c *gin.Context) {
	if err := h.svc.Add(c.Request.Context(), httpx.UserID(c), c.Param("id")); err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *handler) remove(c *gin.Context) {
	if err := h.svc.Remove(c.Request.Context(), httpx.UserID(c), c.Param("id")); err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *handler) listMy(c *gin.Context) {
	items, err := h.svc.ListMy(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if items == nil {
		items = []Watch{}
	}
	httpx.OK(c, gin.H{"items": items})
}
