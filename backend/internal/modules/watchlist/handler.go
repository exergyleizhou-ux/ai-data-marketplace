package watchlist

import (
	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// Register mounts watchlist routes. All require auth (self-scoped).
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}

	u := rg.Group("")
	u.Use(authMW)
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
