package notification

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct{ svc *Service }

// Register mounts notification routes. All require auth (user-scoped).
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	u := rg.Group("/users/me/notifications")
	u.Use(authMW)
	u.GET("", h.list)
	u.GET("/unread-count", h.countUnread)
	u.POST("/read-all",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "notif_read_all", Limit: 20, Window: time.Minute}),
		h.markAllRead)
	u.POST("/:id/read",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "notif_read", Limit: 30, Window: time.Minute}),
		h.markRead)

	p := rg.Group("/users/me/notification-preferences")
	p.Use(authMW)
	p.GET("", h.getPreferences)
	p.PUT("", h.updatePreference)
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

type prefUpdateReq struct {
	Kind         string `json:"kind"`
	EmailEnabled bool   `json:"email_enabled"`
	InAppEnabled bool   `json:"in_app_enabled"`
}

func (h *handler) getPreferences(c *gin.Context) {
	prefs, err := h.svc.GetPreferences(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	if prefs == nil {
		prefs = map[string]NotificationPreference{}
	}
	httpx.OK(c, gin.H{"items": prefs})
}

func (h *handler) updatePreference(c *gin.Context) {
	var req prefUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Kind == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("kind is required"))
		return
	}
	if err := h.svc.UpdatePreference(c.Request.Context(), httpx.UserID(c), req.Kind, req.EmailEnabled, req.InAppEnabled); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"kind": req.Kind, "email_enabled": req.EmailEnabled, "in_app_enabled": req.InAppEnabled})
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
