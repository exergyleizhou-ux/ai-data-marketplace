package workbench

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/workbenchusage"
)

var ErrNotFound = errors.New("workspace not found")
var ErrConflict = errors.New("version conflict")
var ErrAccountInactive = errors.New("account is not active")

type Repository struct {
	db    *pgxpool.Pool
	usage *workbenchusage.Manager
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, usage: workbenchusage.New(db)}
}
func (r *Repository) AccountActive(ctx context.Context, uid string) (bool, error) {
	var active bool
	err := r.db.QueryRow(ctx, `SELECT status='active' FROM users WHERE id=$1`, uid).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}
func (r *Repository) StartQuotaReaper(interval time.Duration) func() {
	return r.usage.StartReaper(interval)
}
func (r *Repository) GetOrCreatePersonalWorkspace(ctx context.Context, uid string) (Workspace, error) {
	var w Workspace
	err := r.db.QueryRow(ctx, `INSERT INTO workbench_workspaces(account_id,slug,display_name) VALUES($1,'personal','Personal') ON CONFLICT(account_id,slug) DO UPDATE SET updated_at=workbench_workspaces.updated_at RETURNING id,account_id,slug,display_name,status`, uid).Scan(&w.ID, &w.AccountID, &w.Slug, &w.DisplayName, &w.Status)
	return w, err
}

func (r *Repository) GetRun(ctx context.Context, o Owner, id string) (Run, error) {
	var v Run
	err := r.db.QueryRow(ctx, `SELECT id,account_id,workspace_id,profile,status,version,title,request,result,error_code,error_message,created_at,started_at,finished_at,updated_at FROM workbench_runs WHERE id=$1 AND account_id=$2 AND workspace_id=$3`, id, o.AccountID, o.WorkspaceID).Scan(&v.ID, &v.AccountID, &v.WorkspaceID, &v.Profile, &v.Status, &v.Version, &v.Title, &v.Request, &v.Result, &v.ErrorCode, &v.ErrorMessage, &v.CreatedAt, &v.StartedAt, &v.FinishedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return v, err
}

func (r *Repository) ListRuns(ctx context.Context, o Owner, limit int) ([]Run, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `SELECT id,account_id,workspace_id,profile,status,version,title,request,result,error_code,error_message,created_at,started_at,finished_at,updated_at FROM workbench_runs WHERE account_id=$1 AND workspace_id=$2 ORDER BY updated_at DESC,id LIMIT $3`, o.AccountID, o.WorkspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Run, 0)
	for rows.Next() {
		var v Run
		if err := rows.Scan(&v.ID, &v.AccountID, &v.WorkspaceID, &v.Profile, &v.Status, &v.Version, &v.Title, &v.Request, &v.Result, &v.ErrorCode, &v.ErrorMessage, &v.CreatedAt, &v.StartedAt, &v.FinishedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateRunCAS prevents concurrent runtime writers from silently overwriting state.
func (r *Repository) UpdateRunCAS(ctx context.Context, o Owner, id string, oldVersion int64, status string, result []byte) (Run, error) {
	var v Run
	err := r.usage.InTx(ctx, func(tx pgx.Tx) error {
		if !workbenchusage.IsTerminal(status) {
			if _, e := r.usage.CheckRunWallTx(ctx, tx, usageOwner(o), id, time.Now()); e != nil {
				return e
			}
		}
		e := tx.QueryRow(ctx, `UPDATE workbench_runs SET status=$5,result=$6,version=version+1,updated_at=now(),finished_at=CASE WHEN $5 IN ('completed','failed','canceled','cancelled') THEN now() ELSE finished_at END WHERE id=$1 AND account_id=$2 AND workspace_id=$3 AND version=$4 RETURNING id,account_id,workspace_id,profile,status,version,title,request,result,error_code,error_message,created_at,started_at,finished_at,updated_at`, id, o.AccountID, o.WorkspaceID, oldVersion, status, result).Scan(&v.ID, &v.AccountID, &v.WorkspaceID, &v.Profile, &v.Status, &v.Version, &v.Title, &v.Request, &v.Result, &v.ErrorCode, &v.ErrorMessage, &v.CreatedAt, &v.StartedAt, &v.FinishedAt, &v.UpdatedAt)
		if e != nil {
			return e
		}
		return r.usage.FinishRunTx(ctx, tx, usageOwner(o), id, status, time.Now())
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, e := r.GetRun(ctx, o, id); errors.Is(e, ErrNotFound) {
			return Run{}, ErrNotFound
		}
		return Run{}, ErrConflict
	}
	return v, err
}

func (r *Repository) AppendEvent(ctx context.Context, e Event) (bool, error) {
	return r.usage.RecordEvent(ctx, usageOwner(Owner{e.AccountID, e.WorkspaceID}), e.RunID, e.ID, e.Seq, e.Type, int64(len(e.Payload)), func(tx pgx.Tx) (bool, error) {
		ct, err := tx.Exec(ctx, `INSERT INTO workbench_events(id,run_id,account_id,workspace_id,seq,type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(run_id,seq) DO NOTHING`, e.ID, e.RunID, e.AccountID, e.WorkspaceID, e.Seq, e.Type, e.Payload, e.CreatedAt)
		return ct.RowsAffected() == 1, err
	})
}

func (r *Repository) Events(ctx context.Context, o Owner, runID string, after int64, limit int) ([]Event, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.Query(ctx, `SELECT id,run_id,account_id,workspace_id,seq,type,payload,created_at FROM workbench_events WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 AND seq>$4 ORDER BY seq LIMIT $5`, runID, o.AccountID, o.WorkspaceID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Event, 0)
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.RunID, &e.AccountID, &e.WorkspaceID, &e.Seq, &e.Type, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ListApprovals(ctx context.Context, o Owner, runID string) ([]Approval, error) {
	rows, err := r.db.Query(ctx, `SELECT approval_id,run_id,tool_call_id,step_id,account_id,workspace_id,owner,risk_level,reason,effects,command,file_scope,remote_target,network_targets,estimated_cost,expected_outputs,args_hash,editable_args,version,created_at,expires_at,decided_at,decided_by,decision,executed_at,execution_id,execution_state FROM workbench_approvals WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 ORDER BY created_at`, runID, o.AccountID, o.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Approval, 0)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ApprovalID, &a.RunID, &a.ToolCallID, &a.StepID, &a.AccountID, &a.WorkspaceID, &a.Owner, &a.RiskLevel, &a.Reason, &a.Effects, &a.Command, &a.FileScope, &a.RemoteTarget, &a.NetworkTargets, &a.EstimatedCost, &a.ExpectedOutputs, &a.ArgsHash, &a.EditableArgs, &a.Version, &a.CreatedAt, &a.ExpiresAt, &a.DecidedAt, &a.DecidedBy, &a.Decision, &a.ExecutedAt, &a.ExecutionID, &a.ExecutionState); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) DecideApproval(ctx context.Context, o Owner, id, actor, decision string, version int64) (Approval, error) {
	var a Approval
	err := r.db.QueryRow(ctx, `UPDATE workbench_approvals SET decision=$5,decided_at=now(),decided_by=$4,version=version+1 WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3 AND version=$6 AND decision IS NULL AND expires_at>now() RETURNING approval_id,run_id,tool_call_id,account_id,workspace_id,owner,risk_level,reason,effects,command,file_scope,remote_target,network_targets,estimated_cost,expected_outputs,args_hash,editable_args,version,created_at,expires_at,decided_at,decided_by,decision,executed_at,execution_id,execution_state`, id, o.AccountID, o.WorkspaceID, actor, decision, version).Scan(&a.ApprovalID, &a.RunID, &a.ToolCallID, &a.AccountID, &a.WorkspaceID, &a.Owner, &a.RiskLevel, &a.Reason, &a.Effects, &a.Command, &a.FileScope, &a.RemoteTarget, &a.NetworkTargets, &a.EstimatedCost, &a.ExpectedOutputs, &a.ArgsHash, &a.EditableArgs, &a.Version, &a.CreatedAt, &a.ExpiresAt, &a.DecidedAt, &a.DecidedBy, &a.Decision, &a.ExecutedAt, &a.ExecutionID, &a.ExecutionState)
	if errors.Is(err, pgx.ErrNoRows) {
		var n int
		e := r.db.QueryRow(ctx, `SELECT 1 FROM workbench_approvals WHERE approval_id=$1 AND account_id=$2 AND workspace_id=$3`, id, o.AccountID, o.WorkspaceID).Scan(&n)
		if errors.Is(e, pgx.ErrNoRows) {
			return Approval{}, ErrNotFound
		}
		return Approval{}, ErrConflict
	}
	return a, err
}

func (r *Repository) ListArtifacts(ctx context.Context, o Owner, runID string) ([]Artifact, error) {
	rows, err := r.db.Query(ctx, `SELECT id,run_id,account_id,workspace_id,name,kind,media_type,object_key,sha256,size_bytes,provenance,metadata,created_at,step_id,tool_call_id,model,input_refs FROM workbench_artifacts WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 ORDER BY created_at,id`, runID, o.AccountID, o.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Artifact, 0)
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.RunID, &a.AccountID, &a.WorkspaceID, &a.Name, &a.Kind, &a.MediaType, &a.ObjectKey, &a.SHA256, &a.SizeBytes, &a.Provenance, &a.Metadata, &a.CreatedAt, &a.StepID, &a.ToolCallID, &a.Model, &a.InputRefs); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) GetArtifact(ctx context.Context, o Owner, id string) (Artifact, error) {
	var a Artifact
	err := r.db.QueryRow(ctx, `SELECT id,run_id,account_id,workspace_id,name,kind,media_type,object_key,sha256,size_bytes,provenance,metadata,created_at,step_id,tool_call_id,model,input_refs FROM workbench_artifacts WHERE id=$1 AND account_id=$2 AND workspace_id=$3`, id, o.AccountID, o.WorkspaceID).Scan(&a.ID, &a.RunID, &a.AccountID, &a.WorkspaceID, &a.Name, &a.Kind, &a.MediaType, &a.ObjectKey, &a.SHA256, &a.SizeBytes, &a.Provenance, &a.Metadata, &a.CreatedAt, &a.StepID, &a.ToolCallID, &a.Model, &a.InputRefs)
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	return a, err
}

func (r *Repository) RecordUsage(ctx context.Context, u Usage) (bool, error) {
	tokens, err := usageTokens(u)
	if err != nil {
		return false, err
	}
	return r.usage.RecordUsage(ctx, usageOwner(Owner{u.AccountID, u.WorkspaceID}), u.RunID, u.EventID, tokens, u.ComputeMillis, u.CostMicrounits, u.OccurredAt, func(tx pgx.Tx) (bool, error) {
		ct, execErr := tx.Exec(ctx, `INSERT INTO workbench_usage(run_id,event_id,account_id,workspace_id,provider,model,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,cost_microunits,compute_millis,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(run_id,event_id) DO NOTHING`, u.RunID, u.EventID, u.AccountID, u.WorkspaceID, u.Provider, u.Model, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.CostMicrounits, u.ComputeMillis, u.OccurredAt)
		return ct.RowsAffected() == 1, execErr
	})
}

func usageTokens(u Usage) (int64, error) {
	var total int64
	for _, value := range []int64{u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens} {
		if value < 0 || value > math.MaxInt64-total {
			return 0, fmt.Errorf("invalid token usage")
		}
		total += value
	}
	return total, nil
}

func usageOwner(o Owner) workbenchusage.Owner {
	return workbenchusage.Owner{AccountID: o.AccountID, WorkspaceID: o.WorkspaceID}
}
func (r *Repository) GetOwned(ctx context.Context, id, uid string) (Workspace, error) {
	var w Workspace
	err := r.db.QueryRow(ctx, `SELECT id,account_id,slug,display_name,status FROM workbench_workspaces WHERE id=$1 AND account_id=$2 AND status='active'`, id, uid).Scan(&w.ID, &w.AccountID, &w.Slug, &w.DisplayName, &w.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return w, err
}
