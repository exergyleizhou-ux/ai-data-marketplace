package moderation

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

// Register mounts moderation routes. Reporting requires auth + rate limiting
// (anti report-spam). The /admin/reports queue requires authMW + opsGate.
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	authed := rg.Group("")
	authed.Use(authMW)
	report := middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "content_report", Limit: 20, Window: time.Minute})
	authed.POST("/questions/:id/report", report, h.reportQuestion)
	authed.POST("/reviews/:id/report", report, h.reportReview)

	admin := rg.Group("/admin/reports")
	admin.Use(authMW, opsGate)
	admin.GET("", h.adminList)
	admin.POST("/:id/resolve", h.adminResolve)
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

func (h *handler) reportQuestion(c *gin.Context) { h.report(c, TargetQuestion) }
func (h *handler) reportReview(c *gin.Context)   { h.report(c, TargetReview) }

func (h *handler) report(c *gin.Context, targetType string) {
	var req reasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	rep, err := h.svc.Report(c.Request.Context(), httpx.UserID(c), targetType, c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rep)
}

func (h *handler) adminList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.ListReports(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if items == nil {
		items = []Report{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type resolveRequest struct {
	Action string `json:"action"` // hide | dismiss
}

func (h *handler) adminResolve(c *gin.Context) {
	var req resolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	rep, err := h.svc.Resolve(c.Request.Context(), c.Param("id"), req.Action, httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rep)
}

// fail maps moderation domain errors to HTTP envelopes.
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidTarget), errors.Is(err, ErrEmptyReason), errors.Is(err, ErrInvalidResolution):
		httpx.Fail(c, httpx.ErrInvalidParam)
	case errors.Is(err, ErrReportNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
