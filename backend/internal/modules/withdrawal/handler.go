package withdrawal

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

func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/sellers/me/withdrawals",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "withdrawal_request", Limit: 5, Window: time.Minute}),
		h.request)
	authed.GET("/sellers/me/withdrawals", h.listMy)

	admin := rg.Group("/admin/withdrawals")
	admin.Use(authMW, opsGate)
	admin.GET("", h.adminList)
	admin.POST("/:id/approve", h.approve)
	admin.POST("/:id/reject", h.reject)
	admin.POST("/:id/complete", h.complete)
}

type withdrawalReq struct {
	AmountCents  int64  `json:"amount_cents"`
	Channel      string `json:"channel"`
	AccountLabel string `json:"account_label"`
}

func (h *handler) request(c *gin.Context) {
	var req withdrawalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	r, err := h.svc.Request(c.Request.Context(), httpx.UserID(c), req.AmountCents, req.Channel, req.AccountLabel)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, r)
}

func (h *handler) listMy(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.ListMy(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Request{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) adminList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.AdminList(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Request{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type opsNoteReq struct {
	Note string `json:"note"`
}

func (h *handler) approve(c *gin.Context) {
	var req opsNoteReq
	_ = c.ShouldBindJSON(&req)
	r, err := h.svc.Approve(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, r)
}

type rejectReq struct {
	Reason string `json:"reason"`
}

func (h *handler) reject(c *gin.Context) {
	var req rejectReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("reason is required"))
		return
	}
	r, err := h.svc.Reject(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, r)
}

func (h *handler) complete(c *gin.Context) {
	var req opsNoteReq
	_ = c.ShouldBindJSON(&req)
	r, err := h.svc.Complete(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, r)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not your withdrawal"))
	case errors.Is(err, ErrAmountInvalid) || errors.Is(err, ErrChannelInvalid) || errors.Is(err, ErrReasonRequired):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrInsufficientBalance):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("insufficient settled balance"))
	case errors.Is(err, ErrBadTransition):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("illegal status transition"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
