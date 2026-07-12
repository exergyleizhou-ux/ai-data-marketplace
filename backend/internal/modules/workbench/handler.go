package workbench

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/workbenchusage"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct {
	s            *Service
	tokenEnabled bool
}

func (h handler) usageSummary(c *gin.Context) {
	accountID := c.Query("account_id")
	workspaceID := c.Query("workspace_id")
	if accountID == "" || workspaceID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	at := time.Now()
	if month := c.Query("month"); month != "" {
		parsed, err := time.Parse("2006-01", month)
		if err != nil {
			httpx.Fail(c, httpx.ErrInvalidParam)
			return
		}
		at = parsed
	}
	summary, err := h.s.runtime.usage.Summary(c, workbenchusage.Owner{AccountID: accountID, WorkspaceID: workspaceID}, at)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, summary)
}

func (h handler) token(c *gin.Context) {
	if !h.tokenEnabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "workbench_unavailable", "message": "workbench identity is not configured"}})
		return
	}
	var q struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := c.ShouldBindJSON(&q); err != nil && !errors.Is(err, io.EOF) {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.s.Issue(c.Request.Context(), httpx.UserID(c), q.WorkspaceID)
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	httpx.OK(c, res)
}

func workspaceID(c *gin.Context) string { return c.Query("workspace_id") }
func failRuntime(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if errors.Is(err, ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "version_conflict", "message": "resource changed; refresh and retry"}})
		return
	}
	httpx.Fail(c, httpx.ErrInternal)
}
func (h handler) runs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	v, e := h.s.Runs(c, httpx.UserID(c), workspaceID(c), limit)
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) run(c *gin.Context) {
	v, e := h.s.Run(c, httpx.UserID(c), workspaceID(c), c.Param("id"))
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) events(c *gin.Context) {
	after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
	v, e := h.s.Events(c, httpx.UserID(c), workspaceID(c), c.Param("id"), after)
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) approvals(c *gin.Context) {
	v, e := h.s.Approvals(c, httpx.UserID(c), workspaceID(c), c.Param("id"))
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) decide(c *gin.Context) {
	var d ApprovalDecision
	if e := c.ShouldBindJSON(&d); e != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	v, e := h.s.Decide(c, httpx.UserID(c), workspaceID(c), c.Param("approvalID"), d)
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) artifacts(c *gin.Context) {
	v, e := h.s.Artifacts(c, httpx.UserID(c), workspaceID(c), c.Param("id"))
	if e != nil {
		failRuntime(c, e)
		return
	}
	httpx.OK(c, v)
}
func (h handler) downloadArtifact(c *gin.Context) {
	a, r, n, e := h.s.OpenArtifact(c, httpx.UserID(c), workspaceID(c), c.Param("artifactID"))
	if e != nil {
		failRuntime(c, e)
		return
	}
	defer r.Close()
	name := strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune("/\\\";\r\n", r) {
			return '_'
		}
		return r
	}, a.Name)
	if name == "" {
		name = "artifact"
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", a.MediaType)
	c.Header("X-Content-SHA256", a.SHA256)
	c.DataFromReader(http.StatusOK, n, a.MediaType, r, nil)
}
