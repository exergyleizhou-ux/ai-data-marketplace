package compliance

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct {
	exportSvc   *ExportService
	deletionSvc *DeletionService
}

func Register(rg *gin.RouterGroup, exportSvc *ExportService, deletionSvc *DeletionService, authMW, opsGate gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{exportSvc: exportSvc, deletionSvc: deletionSvc}

	authed := rg.Group("/users/me")
	authed.Use(authMW)
	authed.POST("/data-export",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "data_export", Limit: 5, Window: time.Minute}),
		h.requestExport)
	authed.GET("/data-export", h.getExportStatus)
	authed.GET("/data-export/download", h.downloadExport)
	authed.POST("/account/deletion",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "account_deletion", Limit: 3, Window: time.Minute}),
		h.requestDeletion)
	authed.DELETE("/account/deletion",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "account_deletion_cancel", Limit: 5, Window: time.Minute}),
		h.cancelDeletion)

	admin := rg.Group("/admin/account-deletions")
	admin.Use(authMW, opsGate)
	admin.GET("", h.adminList)
	admin.POST("/:id/approve", h.approve)
	admin.POST("/:id/reject", h.reject)
	admin.POST("/:id/execute",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "deletion_execute", Limit: 10, Window: time.Minute}),
		h.execute)
}

// --- export ---

func (h *handler) requestExport(c *gin.Context) {
	j, err := h.exportSvc.RequestExport(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) getExportStatus(c *gin.Context) {
	j, err := h.exportSvc.GetExportStatus(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) downloadExport(c *gin.Context) {
	j, err := h.exportSvc.GetExportStatus(c.Request.Context(), httpx.UserID(c))
	if err != nil || j.Status != ExportReady {
		fail(c, ErrExportNotReady)
		return
	}
	// Try local cache first (in-memory), fall back to object storage.
	rc, err := h.exportSvc.OpenExport(c.Request.Context(), j.ID)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage("export file not found"))
		return
	}
	defer rc.Close()

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=\"oasis-data-export-"+time.Now().Format("20060102")+".zip\"")
	c.Status(200)
	_, _ = io.Copy(c.Writer, rc)
}

// --- deletion ---

type deletionReq struct {
	Reason string `json:"reason"`
}

func (h *handler) requestDeletion(c *gin.Context) {
	var req deletionReq
	_ = c.ShouldBindJSON(&req)
	d, err := h.deletionSvc.RequestDeletion(c.Request.Context(), httpx.UserID(c), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) cancelDeletion(c *gin.Context) {
	d, err := h.deletionSvc.CancelDeletion(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": d.ID, "status": d.Status, "ok": true})
}

// --- admin ---

func (h *handler) adminList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.deletionSvc.List(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if items == nil {
		items = []DeletionRequest{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type opsNoteReq struct {
	Note string `json:"note"`
}

func (h *handler) approve(c *gin.Context) {
	var req opsNoteReq
	_ = c.ShouldBindJSON(&req)
	d, err := h.deletionSvc.Approve(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
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
	d, err := h.deletionSvc.Reject(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) execute(c *gin.Context) {
	if err := h.deletionSvc.Execute(c.Request.Context(), httpx.UserID(c), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "deleted", "id": c.Param("id")})
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrExportInProgress) || errors.Is(err, ErrExportNotReady):
		httpx.Fail(c, httpx.ErrConflict.WithMessage(err.Error()))
	case errors.Is(err, ErrDeletionExists) || errors.Is(err, ErrDeletionNotCancelable):
		httpx.Fail(c, httpx.ErrConflict.WithMessage(err.Error()))
	case errors.Is(err, ErrCoolingNotElapsed):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("cooling period has not elapsed"))
	case errors.Is(err, ErrBadTransition):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("illegal status transition"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
