package workbench

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type runtimeHandler struct{ s *Service }

func runtimeAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(secret) < 32 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "runtime_ingest_unavailable", "message": "runtime ingestion is not configured"}})
			return
		}
		got := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ValidRuntimeCredential(got, secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "invalid_runtime_credential", "message": "authentication required"}})
			return
		}
		c.Next()
	}
}
func validOwner(o Owner) bool { return o.AccountID != "" && o.WorkspaceID != "" }
func requireOwner(c *gin.Context, o Owner) bool {
	if validOwner(o) {
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_owner", "message": "account_id and workspace_id are required"}})
	return false
}
func bindRuntime(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "invalid runtime payload"}})
		return false
	}
	return true
}
func runtimeFailure(c *gin.Context, e error) {
	if e == ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "resource not found"}})
	} else if e == ErrConflict {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "version_conflict", "message": "resource changed or is not executable"}})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "runtime_persistence_failed", "message": "runtime state was not persisted"}})
	}
}

func (h runtimeHandler) createRun(c *gin.Context) {
	var in RuntimeRunInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	in.Run.ID = c.Param("id")
	if in.Run.Version < 1 {
		in.Run.Version = 1
	}
	now := time.Now()
	if in.Run.CreatedAt.IsZero() {
		in.Run.CreatedAt = now
	}
	if in.Run.UpdatedAt.IsZero() {
		in.Run.UpdatedAt = now
	}
	v, created, e := h.s.runtime.CreateRun(c, in.Owner, in.Run)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": v, "created": created})
}
func (h runtimeHandler) state(c *gin.Context) {
	var in RuntimeStateInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	v, e := h.s.runtime.UpdateRunCAS(c, in.Owner, c.Param("id"), in.ExpectedVersion, in.Status, in.Result)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"data": v})
}
func (h runtimeHandler) event(c *gin.Context) {
	var in RuntimeEventInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	in.Event.RunID = c.Param("id")
	in.Event.AccountID = in.Owner.AccountID
	in.Event.WorkspaceID = in.Owner.WorkspaceID
	if in.Event.CreatedAt.IsZero() {
		in.Event.CreatedAt = time.Now()
	}
	ok, e := h.s.runtime.AppendEvent(c, in.Event)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"recorded": ok})
}
func (h runtimeHandler) usage(c *gin.Context) {
	var in RuntimeUsageInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	in.Usage.RunID = c.Param("id")
	in.Usage.AccountID = in.Owner.AccountID
	in.Usage.WorkspaceID = in.Owner.WorkspaceID
	if in.Usage.OccurredAt.IsZero() {
		in.Usage.OccurredAt = time.Now()
	}
	ok, e := h.s.runtime.RecordUsage(c, in.Usage)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"recorded": ok})
}
func (h runtimeHandler) approval(c *gin.Context) {
	var in RuntimeApprovalInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	in.Approval.RunID = c.Param("id")
	if in.Approval.Version < 1 {
		in.Approval.Version = 1
	}
	if in.Approval.CreatedAt.IsZero() {
		in.Approval.CreatedAt = time.Now()
	}
	ok, e := h.s.runtime.CreateApproval(c, in.Owner, in.Approval)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"recorded": ok})
}
func (h runtimeHandler) consume(c *gin.Context) {
	var in RuntimeConsumeInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	v, e := h.s.runtime.ConsumeApproval(c, in.Owner, c.Param("approvalID"), in.ArgsHash, in.ExecutionID, in.ExpectedVersion)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"data": v})
}
func (h runtimeHandler) complete(c *gin.Context) {
	var in RuntimeExecutionInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	v, e := h.s.runtime.CompleteApprovalExecution(c, in.Owner, c.Param("approvalID"), in.ExecutionID, in.State, in.ExpectedVersion)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"data": v})
}
func (h runtimeHandler) artifact(c *gin.Context) {
	var in RuntimeArtifactInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	in.Artifact.RunID = c.Param("id")
	v, created, e := h.s.StoreArtifact(c, in)
	if e != nil {
		runtimeFailure(c, e)
		return
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"data": v, "created": created})
}
