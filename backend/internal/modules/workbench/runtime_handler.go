package workbench

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/workbenchusage"
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
	var quota *workbenchusage.LimitError
	if errors.As(e, &quota) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"code": quota.Code, "message": "quota exceeded", "next_action": quota.NextAction}})
	} else if e == ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "resource not found"}})
	} else if e == ErrConflict {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "version_conflict", "message": "resource changed or is not executable"}})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "runtime_persistence_failed", "message": "runtime state was not persisted"}})
	}
}

type quotaAdmissionInput struct {
	Owner     Owner     `json:"owner"`
	StartedAt time.Time `json:"started_at"`
}

func (h runtimeHandler) admit(c *gin.Context) {
	var in quotaAdmissionInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = time.Now()
	}
	p, err := h.s.runtime.usage.AdmitRun(c, usageOwner(in.Owner), c.Param("id"), in.StartedAt)
	if err != nil {
		runtimeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"quota": p}})
}

type quotaCompletionInput struct {
	Owner       Owner     `json:"owner"`
	Status      string    `json:"status"`
	CompletedAt time.Time `json:"completed_at"`
}

func (h runtimeHandler) settle(c *gin.Context) {
	var in quotaCompletionInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	if in.CompletedAt.IsZero() {
		in.CompletedAt = time.Now()
	}
	if !workbenchusage.IsTerminal(in.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "terminal status is required"}})
		return
	}
	if err := h.s.runtime.usage.CompleteRun(c, usageOwner(in.Owner), c.Param("id"), in.Status, in.CompletedAt); err != nil {
		runtimeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"released": true})
}

func (h runtimeHandler) chargeUsage(c *gin.Context) {
	var in RuntimeUsageInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	u := in.Usage
	if u.OccurredAt.IsZero() {
		u.OccurredAt = time.Now()
	}
	tokens, tokenErr := usageTokens(u)
	if tokenErr != nil || u.ComputeMillis < 0 || u.CostMicrounits < 0 || u.EventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "non-negative usage and event_id are required"}})
		return
	}
	recorded, err := h.s.runtime.usage.ChargeUsage(c, usageOwner(in.Owner), c.Param("id"), u.EventID, tokens, u.ComputeMillis, u.CostMicrounits, u.OccurredAt)
	if err != nil {
		runtimeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"recorded": recorded})
}

type quotaArtifactInput struct {
	Owner      Owner  `json:"owner"`
	ArtifactID string `json:"artifact_id"`
	SizeBytes  int64  `json:"size_bytes"`
}

func (h runtimeHandler) reserveArtifact(c *gin.Context) {
	var in quotaArtifactInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	if in.ArtifactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "artifact_id is required"}})
		return
	}
	if err := h.s.runtime.usage.ReserveArtifact(c, usageOwner(in.Owner), c.Param("id"), in.ArtifactID, in.SizeBytes); err != nil {
		runtimeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reserved": true})
}

func (h runtimeHandler) releaseArtifact(c *gin.Context) {
	var in quotaArtifactInput
	if !bindRuntime(c, &in) || !requireOwner(c, in.Owner) {
		return
	}
	if in.ArtifactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "artifact_id is required"}})
		return
	}
	if err := h.s.runtime.usage.ArtifactState(c, usageOwner(in.Owner), c.Param("id"), in.ArtifactID, "released"); err != nil {
		runtimeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"released": true})
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
