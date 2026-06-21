package anomaly

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct{ svc *Service }

func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}
	admin := rg.Group("/admin/anomalies")
	admin.Use(authMW, opsGate)
	admin.GET("", h.list)
	admin.POST("/:id/acknowledge",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "anomaly_ack", Limit: 20, Window: time.Minute}),
		h.acknowledge)
	admin.POST("/:id/resolve",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "anomaly_resolve", Limit: 20, Window: time.Minute}),
		h.resolve)
}

func (h *handler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.List(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if items == nil {
		items = []Anomaly{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type opsNoteReq struct {
	Note string `json:"note"`
}

func (h *handler) acknowledge(c *gin.Context) {
	var req opsNoteReq
	_ = c.ShouldBindJSON(&req)
	a, err := h.svc.Acknowledge(c.Request.Context(), c.Param("id"), httpx.UserID(c), req.Note)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, a)
}

func (h *handler) resolve(c *gin.Context) {
	var req opsNoteReq
	_ = c.ShouldBindJSON(&req)
	a, err := h.svc.Resolve(c.Request.Context(), c.Param("id"), httpx.UserID(c), req.Note)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, a)
}
