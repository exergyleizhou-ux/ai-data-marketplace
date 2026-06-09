package notification

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// Register mounts notification routes. All require auth (user-scoped).
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}

	u := rg.Group("/users/me/notifications")
	u.Use(authMW)
	u.GET("", h.list)
	u.GET("/unread-count", h.countUnread)
	u.POST("/read-all", h.markAllRead)
	u.POST("/:id/read", h.markRead)
}

func (h *handler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.List(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Notification{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) countUnread(c *gin.Context) {
	n, err := h.svc.CountUnread(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"unread": n})
}

func (h *handler) markRead(c *gin.Context) {
	if err := h.svc.MarkRead(c.Request.Context(), httpx.UserID(c), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *handler) markAllRead(c *gin.Context) {
	n, err := h.svc.MarkAllRead(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"marked": n})
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
