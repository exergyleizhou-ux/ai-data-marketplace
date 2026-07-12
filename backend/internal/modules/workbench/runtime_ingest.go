package workbench

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/workbenchusage"
)

type RuntimeRunInput struct {
	Owner Owner `json:"owner"`
	Run   Run   `json:"run"`
}
type RuntimeStateInput struct {
	Owner           Owner           `json:"owner"`
	ExpectedVersion int64           `json:"expected_version"`
	Status          string          `json:"status"`
	Result          json.RawMessage `json:"result"`
}
type RuntimeEventInput struct {
	Owner Owner `json:"owner"`
	Event Event `json:"event"`
}
type RuntimeUsageInput struct {
	Owner Owner `json:"owner"`
	Usage Usage `json:"usage"`
}
type RuntimeApprovalInput struct {
	Owner    Owner    `json:"owner"`
	Approval Approval `json:"approval"`
}
type RuntimeConsumeInput struct {
	Owner           Owner  `json:"owner"`
	ArgsHash        string `json:"args_hash"`
	ExecutionID     string `json:"execution_id"`
	ExpectedVersion int64  `json:"expected_version"`
}
type RuntimeExecutionInput struct {
	Owner           Owner  `json:"owner"`
	ExecutionID     string `json:"execution_id"`
	State           string `json:"state"`
	ExpectedVersion int64  `json:"expected_version"`
}
type RuntimeArtifactInput struct {
	Owner    Owner    `json:"owner"`
	Artifact Artifact `json:"artifact"`
	Content  []byte   `json:"content"`
}

func ValidRuntimeCredential(got, want string) bool {
	if len(want) < 32 || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (r *Repository) CreateRun(ctx context.Context, o Owner, v Run) (Run, bool, error) {
	var created bool
	err := r.usage.InTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_runs WHERE id=$1)`, v.ID).Scan(&exists); e != nil {
			return e
		}
		if exists {
			if !workbenchusage.IsTerminal(v.Status) {
				started := v.CreatedAt
				if v.StartedAt != nil {
					started = *v.StartedAt
				}
				return r.usage.ReserveRunTx(ctx, tx, usageOwner(o), v.ID, started)
			}
			return nil
		}
		ct, e := tx.Exec(ctx, `INSERT INTO workbench_runs(id,account_id,workspace_id,profile,status,version,title,request,result,created_at,started_at,finished_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, o.AccountID, o.WorkspaceID, v.Profile, v.Status, v.Version, v.Title, v.Request, v.Result, v.CreatedAt, v.StartedAt, v.FinishedAt, v.UpdatedAt)
		if e != nil {
			return e
		}
		created = ct.RowsAffected() == 1
		started := v.CreatedAt
		if v.StartedAt != nil {
			started = *v.StartedAt
		}
		if !workbenchusage.IsTerminal(v.Status) {
			return r.usage.ReserveRunTx(ctx, tx, usageOwner(o), v.ID, started)
		}
		return nil
	})
	if err != nil {
		return Run{}, false, err
	}
	got, err := r.GetRun(ctx, o, v.ID)
	if err != nil {
		return Run{}, false, err
	}
	return got, created, nil
}
func (r *Repository) CreateApproval(ctx context.Context, o Owner, a Approval) (bool, error) {
	ct, err := r.db.Exec(ctx, `INSERT INTO workbench_approvals(approval_id,run_id,tool_call_id,step_id,account_id,workspace_id,owner,risk_level,reason,effects,command,file_scope,remote_target,network_targets,estimated_cost,expected_outputs,args_hash,editable_args,version,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT(approval_id) DO NOTHING`, a.ApprovalID, a.RunID, a.ToolCallID, a.StepID, o.AccountID, o.WorkspaceID, a.Owner, a.RiskLevel, a.Reason, a.Effects, a.Command, a.FileScope, a.RemoteTarget, a.NetworkTargets, a.EstimatedCost, a.ExpectedOutputs, a.ArgsHash, a.EditableArgs, a.Version, a.CreatedAt, a.ExpiresAt)
	if err != nil || ct.RowsAffected() == 1 {
		return ct.RowsAffected() == 1, err
	}
	var n int
	if err := r.db.QueryRow(ctx, `SELECT 1 FROM workbench_approvals WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3`, a.ApprovalID, o.AccountID, o.WorkspaceID).Scan(&n); err != nil {
		return false, ErrNotFound
	}
	return false, nil
}
func (r *Repository) ConsumeApproval(ctx context.Context, o Owner, id, argsHash, executionID string, version int64) (Approval, error) {
	var a Approval
	err := r.db.QueryRow(ctx, `UPDATE workbench_approvals SET execution_state='consumed',execution_id=$5,version=version+1 WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3 AND version=$4 AND decision='approved' AND expires_at>now() AND args_hash=$6 AND execution_state='pending' RETURNING approval_id,run_id,tool_call_id,account_id,workspace_id,owner,risk_level,reason,effects,command,file_scope,remote_target,network_targets,estimated_cost,expected_outputs,args_hash,editable_args,version,created_at,expires_at,decided_at,decided_by,decision,executed_at,execution_id,execution_state`, id, o.AccountID, o.WorkspaceID, version, executionID, argsHash).Scan(&a.ApprovalID, &a.RunID, &a.ToolCallID, &a.AccountID, &a.WorkspaceID, &a.Owner, &a.RiskLevel, &a.Reason, &a.Effects, &a.Command, &a.FileScope, &a.RemoteTarget, &a.NetworkTargets, &a.EstimatedCost, &a.ExpectedOutputs, &a.ArgsHash, &a.EditableArgs, &a.Version, &a.CreatedAt, &a.ExpiresAt, &a.DecidedAt, &a.DecidedBy, &a.Decision, &a.ExecutedAt, &a.ExecutionID, &a.ExecutionState)
	if err != nil {
		return Approval{}, r.classifyApprovalMutation(ctx, o, id, err)
	}
	return a, nil
}
func (r *Repository) CompleteApprovalExecution(ctx context.Context, o Owner, id, executionID, state string, version int64) (Approval, error) {
	if state != "executed" && state != "failed" {
		return Approval{}, errors.New("invalid execution state")
	}
	var a Approval
	err := r.db.QueryRow(ctx, `UPDATE workbench_approvals SET execution_state=$6,executed_at=now(),version=version+1 WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3 AND version=$4 AND execution_id=$5 AND execution_state='consumed' RETURNING approval_id,run_id,tool_call_id,account_id,workspace_id,owner,risk_level,reason,effects,command,file_scope,remote_target,network_targets,estimated_cost,expected_outputs,args_hash,editable_args,version,created_at,expires_at,decided_at,decided_by,decision,executed_at,execution_id,execution_state`, id, o.AccountID, o.WorkspaceID, version, executionID, state).Scan(&a.ApprovalID, &a.RunID, &a.ToolCallID, &a.AccountID, &a.WorkspaceID, &a.Owner, &a.RiskLevel, &a.Reason, &a.Effects, &a.Command, &a.FileScope, &a.RemoteTarget, &a.NetworkTargets, &a.EstimatedCost, &a.ExpectedOutputs, &a.ArgsHash, &a.EditableArgs, &a.Version, &a.CreatedAt, &a.ExpiresAt, &a.DecidedAt, &a.DecidedBy, &a.Decision, &a.ExecutedAt, &a.ExecutionID, &a.ExecutionState)
	if err != nil {
		return Approval{}, r.classifyApprovalMutation(ctx, o, id, err)
	}
	return a, nil
}
func (r *Repository) classifyApprovalMutation(ctx context.Context, o Owner, id string, err error) error {
	if err == nil {
		return nil
	}
	var n int
	if e := r.db.QueryRow(ctx, `SELECT 1 FROM workbench_approvals WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3`, id, o.AccountID, o.WorkspaceID).Scan(&n); e != nil {
		return ErrNotFound
	}
	return ErrConflict
}
func (r *Repository) InsertArtifact(ctx context.Context, a Artifact) (bool, error) {
	ct, err := r.db.Exec(ctx, `INSERT INTO workbench_artifacts(id,run_id,account_id,workspace_id,name,kind,media_type,object_key,sha256,size_bytes,provenance,metadata,created_at,step_id,tool_call_id,model,input_refs) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(id) DO NOTHING`, a.ID, a.RunID, a.AccountID, a.WorkspaceID, a.Name, a.Kind, a.MediaType, a.ObjectKey, a.SHA256, a.SizeBytes, a.Provenance, a.Metadata, a.CreatedAt, a.StepID, a.ToolCallID, a.Model, a.InputRefs)
	return ct.RowsAffected() == 1, err
}

func (r *Repository) DeleteArtifact(ctx context.Context, o Owner, id string) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM workbench_artifacts WHERE id=$1 AND account_id=$2 AND workspace_id=$3`, id, o.AccountID, o.WorkspaceID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) StoreArtifact(ctx context.Context, in RuntimeArtifactInput) (Artifact, bool, error) {
	if s.objects == nil {
		return Artifact{}, false, errors.New("object storage unavailable")
	}
	a := in.Artifact
	a.AccountID = in.Owner.AccountID
	a.WorkspaceID = in.Owner.WorkspaceID
	if got, e := s.runtime.GetArtifact(ctx, in.Owner, a.ID); e == nil {
		return got, false, nil
	}
	if e := s.runtime.usage.ReserveArtifact(ctx, usageOwner(in.Owner), a.RunID, a.ID, int64(len(in.Content))); e != nil {
		return Artifact{}, false, e
	}
	committed := false
	releaseReservation := true
	defer func() {
		if !committed && releaseReservation {
			_ = s.runtime.usage.ArtifactState(context.Background(), usageOwner(in.Owner), a.RunID, a.ID, "released")
		}
	}()
	key, e := ArtifactObjectKey(in.Owner, a.RunID, a.ID)
	if e != nil {
		return Artifact{}, false, e
	}
	upload, e := s.objects.InitMultipart(ctx, key)
	if e != nil {
		return Artifact{}, false, e
	}
	abort := true
	defer func() {
		if abort {
			_ = s.objects.Abort(ctx, upload)
		}
	}()
	if _, e = s.objects.PutPart(ctx, upload, 1, bytes.NewReader(in.Content)); e != nil {
		return Artifact{}, false, e
	}
	obj, e := s.objects.CompleteMultipart(ctx, upload)
	if e != nil {
		return Artifact{}, false, e
	}
	abort = false
	a.ObjectKey = obj.Key
	a.SHA256 = obj.SHA256
	a.SizeBytes = obj.Size
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	created, e := s.runtime.InsertArtifact(ctx, a)
	if e != nil {
		_ = s.objects.Delete(ctx, key)
		return Artifact{}, false, e
	}
	if !created {
		if e = s.runtime.usage.ArtifactState(ctx, usageOwner(in.Owner), a.RunID, a.ID, "committed"); e != nil {
			return Artifact{}, false, e
		}
		committed = true
		got, err := s.runtime.GetArtifact(ctx, in.Owner, a.ID)
		return got, false, err
	}
	if e = s.runtime.usage.ArtifactState(ctx, usageOwner(in.Owner), a.RunID, a.ID, "committed"); e != nil {
		releaseReservation = false
		metaErr := s.runtime.DeleteArtifact(ctx, in.Owner, a.ID)
		objectErr := s.objects.Delete(ctx, key)
		if metaErr == nil && objectErr == nil {
			releaseReservation = true
			_ = s.runtime.usage.ArtifactState(context.Background(), usageOwner(in.Owner), a.RunID, a.ID, "released")
		}
		return Artifact{}, false, fmt.Errorf("commit artifact quota: %w (metadata cleanup: %v, object cleanup: %v)", e, metaErr, objectErr)
	}
	committed = true
	return a, true, nil
}
