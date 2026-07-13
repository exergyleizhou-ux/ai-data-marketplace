// Package workbenchusage implements hard Workbench quotas. PostgreSQL row locks
// are the authority; cache outages therefore cannot weaken enforcement.
package workbenchusage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
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

const LeaseDuration = 2 * time.Minute

var ErrReservationNotFound = errors.New("quota reservation not found")

func New(db *pgxpool.Pool) *Manager                   { return &Manager{db: db, now: time.Now} }
func (m *Manager) SetClockForTest(f func() time.Time) { m.now = f }

func policy(ctx context.Context, tx pgx.Tx, o Owner) (Policy, error) {
	// Serialize account-wide concurrency across every workspace belonging to
	// the same user before taking the workspace policy row lock.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 9283746))`, o.AccountID); err != nil {
		return Policy{}, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO workbench_quota_policies(account_id,workspace_id) VALUES($1,$2) ON CONFLICT(workspace_id) DO NOTHING`, o.AccountID, o.WorkspaceID)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	err = tx.QueryRow(ctx, `SELECT user_concurrent_runs,workspace_concurrent_runs,monthly_tokens,monthly_compute_millis,storage_bytes,artifact_total_bytes,artifact_single_bytes,run_wall_millis,run_max_steps,run_max_events,event_max_bytes FROM workbench_quota_policies WHERE account_id=$1 AND workspace_id=$2 FOR UPDATE`, o.AccountID, o.WorkspaceID).Scan(&p.UserConcurrent, &p.WorkspaceConcurrent, &p.MonthlyTokens, &p.MonthlyCompute, &p.StorageBytes, &p.ArtifactTotal, &p.ArtifactSingle, &p.RunWallMillis, &p.RunMaxSteps, &p.RunMaxEvents, &p.EventMaxBytes)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT min(user_concurrent_runs) FROM workbench_quota_policies WHERE account_id=$1`, o.AccountID).Scan(&p.UserConcurrent)
	}
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
	now := m.now()
	if err = m.reconcileExpiredTx(ctx, tx, o, now); err != nil {
		return err
	}
	var existingState string
	err = tx.QueryRow(ctx, `SELECT state FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3`, runID, o.AccountID, o.WorkspaceID).Scan(&existingState)
	if err == nil {
		if existingState == "active" {
			return nil
		}
		return limit("quota_run_lease_expired", "start_new_run")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	month := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	var usedTokens, usedCompute, storage int64
	err = tx.QueryRow(ctx, `SELECT
		COALESCE((SELECT tokens FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3),0),
		COALESCE((SELECT compute_millis FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3),0),
		COALESCE((SELECT sum(size_bytes) FROM workbench_artifact_reservations WHERE account_id=$1 AND workspace_id=$2 AND state!='released'),0)`, o.AccountID, o.WorkspaceID, month).Scan(&usedTokens, &usedCompute, &storage)
	if err != nil {
		return err
	}
	if usedTokens >= p.MonthlyTokens {
		return limit("quota_monthly_tokens", "retry_next_month")
	}
	if usedCompute >= p.MonthlyCompute {
		return limit("quota_monthly_compute", "retry_next_month")
	}
	if storage >= p.StorageBytes {
		return limit("quota_storage", "delete_artifacts")
	}
	var users, workspace int64
	err = tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM workbench_run_reservations WHERE account_id=$1 AND state='active'),
		(SELECT count(*) FROM workbench_run_reservations WHERE workspace_id=$2 AND state='active')`, o.AccountID, o.WorkspaceID).Scan(&users, &workspace)
	if err != nil {
		return err
	}
	if users >= p.UserConcurrent {
		return limit("quota_user_concurrent_runs", "wait_for_run")
	}
	if workspace >= p.WorkspaceConcurrent {
		return limit("quota_workspace_concurrent_runs", "wait_for_run")
	}
	_, err = tx.Exec(ctx, `INSERT INTO workbench_run_reservations(run_id,account_id,workspace_id,started_at,last_heartbeat_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, runID, o.AccountID, o.WorkspaceID, started, now, now.Add(LeaseDuration))
	return err
}

// AdmitRun is the independent admission contract used when Lumen and Oasis
// share PostgreSQL and the durable run row therefore already exists.
func (m *Manager) AdmitRun(ctx context.Context, o Owner, runID string, started time.Time) (Policy, time.Time, error) {
	var p Policy
	var expires time.Time
	err := m.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		p, err = policy(ctx, tx, o)
		if err != nil {
			return err
		}
		if err = m.ReserveRunTx(ctx, tx, o, runID, started); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT expires_at FROM workbench_run_reservations WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 AND state='active'`, runID, o.AccountID, o.WorkspaceID).Scan(&expires)
	})
	return p, expires, err
}

func (m *Manager) Heartbeat(ctx context.Context, o Owner, runID string) (time.Time, error) {
	var expires time.Time
	err := m.InTx(ctx, func(tx pgx.Tx) error {
		_, err := policy(ctx, tx, o)
		if err != nil {
			return err
		}
		now := m.now()
		if err = m.reconcileExpiredTx(ctx, tx, o, now); err != nil {
			return err
		}
		expires = now.Add(LeaseDuration)
		ct, err := tx.Exec(ctx, `UPDATE workbench_run_reservations SET last_heartbeat_at=$4,expires_at=$5 WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 AND state='active'`, runID, o.AccountID, o.WorkspaceID, now, expires)
		if err != nil {
			return err
		}
		if ct.RowsAffected() != 1 {
			return limit("quota_run_lease_expired", "start_new_run")
		}
		return nil
	})
	return expires, err
}

// Reconcile releases and meters every expired lease. It is safe to run from
// multiple replicas because the same account advisory lock and row locks used
// by admission serialize the transition.
func (m *Manager) Reconcile(ctx context.Context) error {
	rows, err := m.db.Query(ctx, `SELECT DISTINCT account_id,workspace_id FROM workbench_run_reservations WHERE state='active' AND expires_at<=$1`, m.now())
	if err != nil {
		return err
	}
	var owners []Owner
	for rows.Next() {
		var o Owner
		if err = rows.Scan(&o.AccountID, &o.WorkspaceID); err != nil {
			rows.Close()
			return err
		}
		owners = append(owners, o)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, o := range owners {
		err = m.InTx(ctx, func(tx pgx.Tx) error {
			if _, e := policy(ctx, tx, o); e != nil {
				return e
			}
			return m.reconcileExpiredTx(ctx, tx, o, m.now())
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) StartReaper(interval time.Duration) func() {
	if interval <= 0 {
		interval = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Reconcile(ctx)
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}

func (m *Manager) reconcileExpiredTx(ctx context.Context, tx pgx.Tx, o Owner, at time.Time) error {
	rows, err := tx.Query(ctx, `SELECT r.run_id,r.workspace_id,r.started_at,r.expires_at,q.run_wall_millis,q.monthly_compute_millis FROM workbench_run_reservations r JOIN workbench_quota_policies q ON q.account_id=r.account_id AND q.workspace_id=r.workspace_id WHERE r.account_id=$1 AND r.state='active' AND r.expires_at<=$2 FOR UPDATE OF r`, o.AccountID, at)
	if err != nil {
		return err
	}
	type expired struct {
		id, workspace    string
		started, expires time.Time
		wall, monthly    int64
	}
	var all []expired
	for rows.Next() {
		var v expired
		if err = rows.Scan(&v.id, &v.workspace, &v.started, &v.expires, &v.wall, &v.monthly); err != nil {
			rows.Close()
			return err
		}
		all = append(all, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, v := range all {
		ep := Policy{RunWallMillis: v.wall, MonthlyCompute: v.monthly}
		eo := Owner{AccountID: o.AccountID, WorkspaceID: v.workspace}
		if err = m.settleReservationTx(ctx, tx, eo, ep, v.id, v.started, v.expires); err != nil {
			return err
		}
	}
	return nil
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
	return m.settleReservationTx(ctx, tx, o, p, runID, started, at)
}

func (m *Manager) settleReservationTx(ctx context.Context, tx pgx.Tx, o Owner, p Policy, runID string, started, at time.Time) error {
	compute := at.Sub(started).Milliseconds()
	if compute < 0 {
		compute = 0
	}
	if compute > p.RunWallMillis {
		compute = p.RunWallMillis
	}
	month := time.Date(at.UTC().Year(), at.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	_, err := tx.Exec(ctx, `INSERT INTO workbench_monthly_ledgers(account_id,workspace_id,month_start,compute_millis) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,workspace_id,month_start) DO UPDATE SET compute_millis=workbench_monthly_ledgers.compute_millis+LEAST(EXCLUDED.compute_millis,GREATEST(0,$5-workbench_monthly_ledgers.compute_millis)),updated_at=now()`, o.AccountID, o.WorkspaceID, month, compute, p.MonthlyCompute)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workbench_run_reservations SET state='released',released_at=$4,expires_at=$4 WHERE run_id=$1 AND account_id=$2 AND workspace_id=$3 AND state='active'`, runID, o.AccountID, o.WorkspaceID, at)
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
		if eventType == "step" || eventType == "step_started" || eventType == "tool_dispatch" {
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
		var usedT, usedC, usedCost int64
		e = tx.QueryRow(ctx, `SELECT tokens,compute_millis,cost_microunits FROM workbench_monthly_ledgers WHERE account_id=$1 AND workspace_id=$2 AND month_start=$3 FOR UPDATE`, o.AccountID, o.WorkspaceID, month).Scan(&usedT, &usedC, &usedCost)
		if e != nil {
			return e
		}
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workbench_usage_debits WHERE run_id=$1 AND event_id=$2)`, runID, eventID).Scan(&exists)
		if e != nil || exists {
			return e
		}
		if exceeds(usedT, tokens, p.MonthlyTokens) {
			return limit("quota_monthly_tokens", "retry_next_month")
		}
		if exceeds(usedC, compute, p.MonthlyCompute) {
			return limit("quota_monthly_compute", "retry_next_month")
		}
		if exceeds(usedCost, cost, math.MaxInt64) {
			return limit("quota_cost_overflow", "contact_support")
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
		if exceeds(runTotal, size, p.ArtifactTotal) {
			return limit("quota_artifact_total", "reduce_artifact")
		}
		if exceeds(allTotal, size, p.StorageBytes) {
			return limit("quota_storage", "delete_artifacts")
		}
		_, e = tx.Exec(ctx, `INSERT INTO workbench_artifact_reservations(artifact_id,run_id,account_id,workspace_id,size_bytes) VALUES($1,$2,$3,$4,$5)`, id, runID, o.AccountID, o.WorkspaceID, size)
		return e
	})
}

func exceeds(used, add, maximum int64) bool {
	return used < 0 || add < 0 || maximum < 0 || used > maximum || add > maximum-used || used > math.MaxInt64-add
}
func (m *Manager) ArtifactState(ctx context.Context, o Owner, runID, id, state string) error {
	if state != "committed" && state != "released" {
		return fmt.Errorf("invalid artifact state")
	}
	ct, err := m.db.Exec(ctx, `UPDATE workbench_artifact_reservations SET state=$5,updated_at=now() WHERE artifact_id=$1 AND run_id=$2 AND account_id=$3 AND workspace_id=$4 AND state='reserved'`, id, runID, o.AccountID, o.WorkspaceID, state)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 1 {
		return nil
	}
	var current string
	err = m.db.QueryRow(ctx, `SELECT state FROM workbench_artifact_reservations WHERE artifact_id=$1 AND run_id=$2 AND account_id=$3 AND workspace_id=$4`, id, runID, o.AccountID, o.WorkspaceID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return err
	}
	if current == state {
		return nil
	}
	return fmt.Errorf("artifact reservation is %s, cannot transition to %s", current, state)
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
