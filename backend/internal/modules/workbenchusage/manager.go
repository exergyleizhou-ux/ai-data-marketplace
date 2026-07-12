// Package workbenchusage implements hard Workbench quotas. PostgreSQL row locks
// are the authority; cache outages therefore cannot weaken enforcement.
package workbenchusage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Owner struct{ AccountID, WorkspaceID string }

type LimitError struct {
	Code, NextAction string
}

func (e *LimitError) Error() string { return e.Code }

func limit(code, action string) error { return &LimitError{Code: code, NextAction: action} }

type Policy struct {
	UserConcurrent      int64 `json:"user_concurrent_runs"`
	WorkspaceConcurrent int64 `json:"workspace_concurrent_runs"`
	MonthlyTokens       int64 `json:"monthly_tokens"`
	MonthlyCompute      int64 `json:"monthly_compute_millis"`
	StorageBytes        int64 `json:"storage_bytes"`
	ArtifactTotal       int64 `json:"artifact_total_bytes"`
	ArtifactSingle      int64 `json:"artifact_single_bytes"`
	RunWallMillis       int64 `json:"run_wall_millis"`
	RunMaxSteps         int64 `json:"run_max_steps"`
	RunMaxEvents        int64 `json:"run_max_events"`
	EventMaxBytes       int64 `json:"event_max_bytes"`
}

type Manager struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func New(db *pgxpool.Pool) *Manager                   { return &Manager{db: db, now: time.Now} }
func (m *Manager) SetClockForTest(f func() time.Time) { m.now = f }

func policy(ctx context.Context, tx pgx.Tx, o Owner) (Policy, error) {
	_, err := tx.Exec(ctx, `INSERT INTO workbench_quota_policies(account_id,workspace_id) VALUES($1,$2) ON CONFLICT(workspace_id) DO NOTHING`, o.AccountID, o.WorkspaceID)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	err = tx.QueryRow(ctx, `SELECT user_concurrent_runs,workspace_concurrent_runs,monthly_tokens,monthly_compute_millis,storage_bytes,artifact_total_bytes,artifact_single_bytes,run_wall_millis,run_max_steps,run_max_events,event_max_bytes FROM workbench_quota_policies WHERE account_id=$1 AND workspace_id=$2 FOR UPDATE`, o.AccountID, o.WorkspaceID).Scan(&p.UserConcurrent, &p.WorkspaceConcurrent, &p.MonthlyTokens, &p.MonthlyCompute, &p.StorageBytes, &p.ArtifactTotal, &p.ArtifactSingle, &p.RunWallMillis, &p.RunMaxSteps, &p.RunMaxEvents, &p.EventMaxBytes)
	return p, err
}

func (m *Manager) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	// The workspace policy row is the serialization point for every hard quota
	// mutation. Redis may accelerate reads, but is never authoritative.
	tx, err := m.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Manager) ReserveRunTx(ctx context.Context, tx pgx.Tx, o Owner, runID string, started time.Time) error {
	p, err := policy(ctx, tx, o)
	if err != nil {
		return err
	}
	var reserved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3)`, runID, o.AccountID, o.WorkspaceID).Scan(&reserved); err != nil || reserved {
		return err
	}
	var users, workspace int64
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE account_id=$1),count(*) FROM workbench_run_reservations WHERE workspace_id=$2 AND state='active'`, o.AccountID, o.WorkspaceID).Scan(&users, &workspace)
	if err != nil {
		return err
	}
	if users >= p.UserConcurrent {
		return limit("quota_user_concurrent_runs", "wait_for_run")
	}
	if workspace >= p.WorkspaceConcurrent {
		return limit("quota_workspace_concurrent_runs", "wait_for_run")
	}
	_, err = tx.Exec(ctx, `INSERT INTO workbench_run_reservations(run_id,account_id,workspace_id,started_at) VALUES($1,$2,$3,$4)`, runID, o.AccountID, o.WorkspaceID, started)
	return err
}

// AdmitRun is the independent admission contract used when Lumen and Oasis
// share PostgreSQL and the durable run row therefore already exists.
func (m *Manager) AdmitRun(ctx context.Context, o Owner, runID string, started time.Time) (Policy, error) {
	var p Policy
	err := m.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		p, err = policy(ctx, tx, o)
		if err != nil {
			return err
		}
		return m.ReserveRunTx(ctx, tx, o, runID, started)
	})
	return p, err
}

func (m *Manager) CompleteRun(ctx context.Context, o Owner, runID, status string, at time.Time) error {
	return m.InTx(ctx, func(tx pgx.Tx) error { return m.FinishRunTx(ctx, tx, o, runID, status, at) })
}

func IsTerminal(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled":
		return true
	}
	return false
}
func (m *Manager) FinishRunTx(ctx context.Context, tx pgx.Tx, o Owner, runID, status string, at time.Time) error {
	if !IsTerminal(status) {
		return nil
	}
	p, err := policy(ctx, tx, o)
	if err != nil {
		return err
	}
	var started time.Time
	var state string
	err = tx.QueryRow(ctx, `SELECT started_at,state FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 FOR UPDATE`, runID, o.AccountID, o.WorkspaceID).Scan(&started, &state)
	if errors.Is(err, pgx.ErrNoRows) || state == "released" {
		return nil
	}
	if err != nil {
		return err
	}
	compute := at.Sub(started).Milliseconds()
	if compute < 0 {
		compute = 0
	}
	if compute > p.RunWallMillis {
		compute = p.RunWallMillis
	}
	month := time.Date(at.UTC().Year(), at.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	_, err = tx.Exec(ctx, `INSERT INTO workbench_monthly_ledgers(account_id,workspace_id,month_start,compute_millis) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,workspace_id,month_start) DO UPDATE SET compute_millis=workbench_monthly_ledgers.compute_millis+EXCLUDED.compute_millis,updated_at=now()`, o.AccountID, o.WorkspaceID, month, compute)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workbench_run_reservations SET state='released',released_at=$4 WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3`, runID, o.AccountID, o.WorkspaceID, at)
	return err
}

func (m *Manager) CheckRunWallTx(ctx context.Context, tx pgx.Tx, o Owner, runID string, at time.Time) (Policy, error) {
	p, err := policy(ctx, tx, o)
	if err != nil {
		return p, err
	}
	var started time.Time
	if err = tx.QueryRow(ctx, `SELECT started_at FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3`, runID, o.AccountID, o.WorkspaceID).Scan(&started); err != nil {
		return p, err
	}
	if at.Sub(started).Milliseconds() > p.RunWallMillis {
		return p, limit("quota_run_wall_time", "start_new_run")
	}
	return p, nil
}

func (m *Manager) RecordEvent(ctx context.Context, o Owner, runID, eventID string, seq int64, eventType string, size int64, insert func(pgx.Tx) (bool, error)) (bool, error) {
	if size < 0 {
		return false, fmt.Errorf("negative event size")
	}
	var recorded bool
	err := m.InTx(ctx, func(tx pgx.Tx) error {
		p, e := m.CheckRunWallTx(ctx, tx, o, runID, m.now())
		if e != nil {
			return e
		}
		if size > p.EventMaxBytes {
			return limit("quota_event_size", "reduce_event")
		}
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_events WHERE run_id=$1 AND (seq=$2 OR id=$3))`, runID, seq, eventID).Scan(&exists)
		if e != nil {
			return e
		}
		if exists {
			return nil
		}
		var count, steps int64
		e = tx.QueryRow(ctx, `SELECT event_count,step_count FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 FOR UPDATE`, runID, o.AccountID, o.WorkspaceID).Scan(&count, &steps)
		if e != nil {
			return e
		}
		if count >= p.RunMaxEvents {
			return limit("quota_run_events", "start_new_run")
		}
		step := int64(0)
		if eventType == "step" || eventType == "step_started" {
			step = 1
		}
		if steps+step > p.RunMaxSteps {
			return limit("quota_run_steps", "start_new_run")
		}
		recorded, e = insert(tx)
		if e != nil {
			return e
		}
		if recorded {
			_, e = tx.Exec(ctx, `UPDATE workbench_run_reservations SET event_count=event_count+1,step_count=step_count+$2 WHERE run_id=$1 AND account_id=$3 AND workspace_id=$4`, runID, step, o.AccountID, o.WorkspaceID)
		}
		return e
	})
	return recorded, err
}

func (m *Manager) RecordUsage(ctx context.Context, o Owner, runID, eventID string, tokens, compute, cost int64, occurred time.Time, insert func(pgx.Tx) (bool, error)) (bool, error) {
	if tokens < 0 || compute < 0 || cost < 0 {
		return false, fmt.Errorf("negative usage")
	}
	var recorded bool
	err := m.InTx(ctx, func(tx pgx.Tx) error {
		p, e := policy(ctx, tx, o)
		if e != nil {
			return e
		}
		month := time.Date(occurred.UTC().Year(), occurred.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		_, e = tx.Exec(ctx, `INSERT INTO workbench_monthly_ledgers(account_id,workspace_id,month_start) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, o.AccountID, o.WorkspaceID, month)
		if e != nil {
			return e
		}
		var usedT, usedC int64
		e = tx.QueryRow(ctx, `SELECT tokens,compute_millis FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3 FOR UPDATE`, o.AccountID, o.WorkspaceID, month).Scan(&usedT, &usedC)
		if e != nil {
			return e
		}
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_usage_debits WHERE run_id=$1 AND event_id=$2)`, runID, eventID).Scan(&exists)
		if e != nil || exists {
			return e
		}
		if usedT+tokens > p.MonthlyTokens {
			return limit("quota_monthly_tokens", "retry_next_month")
		}
		if usedC+compute > p.MonthlyCompute {
			return limit("quota_monthly_compute", "retry_next_month")
		}
		recorded, e = insert(tx)
		if e != nil {
			return e
		}
		if recorded {
			_, e = tx.Exec(ctx, `INSERT INTO workbench_usage_debits(run_id,event_id,account_id,workspace_id,month_start,tokens,compute_millis,cost_microunits) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, runID, eventID, o.AccountID, o.WorkspaceID, month, tokens, compute, cost)
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE workbench_monthly_ledgers SET tokens=tokens+$4,compute_millis=compute_millis+$5,cost_microunits=cost_microunits+$6,updated_at=now() WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3`, o.AccountID, o.WorkspaceID, month, tokens, compute, cost)
			}
		}
		return e
	})
	return recorded, err
}

// ChargeUsage debits an already-persisted usage event. The debit table, rather
// than the caller's delivery attempt, is the idempotency authority.
func (m *Manager) ChargeUsage(ctx context.Context, o Owner, runID, eventID string, tokens, compute, cost int64, occurred time.Time) (bool, error) {
	return m.RecordUsage(ctx, o, runID, eventID, tokens, compute, cost, occurred, func(pgx.Tx) (bool, error) { return true, nil })
}

func (m *Manager) ReserveArtifact(ctx context.Context, o Owner, runID, id string, size int64) error {
	if size < 0 {
		return fmt.Errorf("negative artifact size")
	}
	return m.InTx(ctx, func(tx pgx.Tx) error {
		p, e := policy(ctx, tx, o)
		if e != nil {
			return e
		}
		if size > p.ArtifactSingle {
			return limit("quota_artifact_single", "reduce_artifact")
		}
		if _, e = m.CheckRunWallTx(ctx, tx, o, runID, m.now()); e != nil {
			return e
		}
		var existingRun, existingAccount, existingWorkspace, existingState string
		var existingSize int64
		e = tx.QueryRow(ctx, `SELECT run_id,account_id,workspace_id,size_bytes,state FROM workbench_artifact_reservations WHERE artifact_id=$1`, id).Scan(&existingRun, &existingAccount, &existingWorkspace, &existingSize, &existingState)
		if e == nil {
			if existingRun == runID && existingAccount == o.AccountID && existingWorkspace == o.WorkspaceID && existingSize == size && existingState != "released" {
				return nil
			}
			return fmt.Errorf("artifact reservation conflict")
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var persisted bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_artifacts WHERE id=$1 AND run_id=$2 AND account_id=$3 AND workspace_id=$4 AND size_bytes=$5)`, id, runID, o.AccountID, o.WorkspaceID, size).Scan(&persisted)
		if e != nil || persisted {
			return e
		}
		var runTotal, allTotal int64
		e = tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes) FILTER(WHERE run_id=$1 AND state!='released'),0),COALESCE(sum(size_bytes) FILTER(WHERE state!='released'),0) FROM workbench_artifact_reservations WHERE workspace_id=$2`, runID, o.WorkspaceID).Scan(&runTotal, &allTotal)
		if e != nil {
			return e
		}
		if runTotal+size > p.ArtifactTotal {
			return limit("quota_artifact_total", "reduce_artifact")
		}
		if allTotal+size > p.StorageBytes {
			return limit("quota_storage", "delete_artifacts")
		}
		_, e = tx.Exec(ctx, `INSERT INTO workbench_artifact_reservations(artifact_id,run_id,account_id,workspace_id,size_bytes) VALUES($1,$2,$3,$4,$5)`, id, runID, o.AccountID, o.WorkspaceID, size)
		return e
	})
}
func (m *Manager) ArtifactState(ctx context.Context, o Owner, runID, id, state string) error {
	if state != "committed" && state != "released" {
		return fmt.Errorf("invalid artifact state")
	}
	_, e := m.db.Exec(ctx, `UPDATE workbench_artifact_reservations SET state=$5 WHERE artifact_id=$1 AND run_id=$2 AND account_id=$3 AND workspace_id=$4 AND state='reserved'`, id, runID, o.AccountID, o.WorkspaceID, state)
	return e
}

type Summary struct {
	AccountID                                                       string    `json:"account_id"`
	WorkspaceID                                                     string    `json:"workspace_id"`
	MonthStart                                                      time.Time `json:"month_start"`
	Tokens, ComputeMillis, CostMicrounits, StorageBytes, ActiveRuns int64
}

func (m *Manager) Summary(ctx context.Context, o Owner, at time.Time) (Summary, error) {
	var s Summary
	s.AccountID = o.AccountID
	s.WorkspaceID = o.WorkspaceID
	s.MonthStart = time.Date(at.UTC().Year(), at.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	e := m.db.QueryRow(ctx, `SELECT COALESCE((SELECT tokens FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3),0),COALESCE((SELECT compute_millis FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3),0),COALESCE((SELECT cost_microunits FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3),0),COALESCE((SELECT sum(size_bytes) FROM workbench_artifact_reservations WHERE account_id=$1 AND workspace_id=$2 AND state='committed'),0),COALESCE((SELECT count(*) FROM workbench_run_reservations WHERE account_id=$1 AND workspace_id=$2 AND state='active'),0)`, o.AccountID, o.WorkspaceID, s.MonthStart).Scan(&s.Tokens, &s.ComputeMillis, &s.CostMicrounits, &s.StorageBytes, &s.ActiveRuns)
	return s, e
}
