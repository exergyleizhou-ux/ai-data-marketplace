package auditlog

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// Register mounts audit-log routes. Requires ops role (gate injected by caller).
func Register(rg *gin.RouterGroup, svc *Service, authMW, opsGate gin.HandlerFunc) {
	h := &handler{svc: svc}

	admin := rg.Group("/admin/audit-logs")
	admin.Use(authMW, opsGate)
	admin.GET("", h.list)
}

func (h *handler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.List(c.Request.Context(), ListFilter{
		ActorID:      c.Query("actor"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
		From:         c.Query("from"),
		To:           c.Query("to"),
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if items == nil {
		items = []LogEntry{}
	}
	resp := gin.H{"items": items, "limit": limit, "offset": offset}
	if len(items) == limit {
		resp["next_offset"] = offset + limit
	}
	httpx.OK(c, resp)
}
