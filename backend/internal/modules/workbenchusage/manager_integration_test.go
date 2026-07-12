package workbenchusage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformdb "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func quotaFixture(t *testing.T) (*Manager, *pgxpool.Pool, Owner) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	if err := platformdb.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_lock(9283746)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = lock.Exec(context.Background(), `SELECT pg_advisory_unlock(9283746)`); lock.Release() })
	_, err = pool.Exec(ctx, `TRUNCATE workbench_usage_debits,workbench_artifact_reservations,workbench_run_reservations,workbench_monthly_ledgers,workbench_quota_policies,workbench_usage,workbench_artifacts,workbench_approvals,workbench_events,workbench_runs,workbench_workspaces CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	var owner Owner
	account := fmt.Sprintf("quota-%d@example.test", time.Now().UnixNano())
	if err = pool.QueryRow(ctx, `INSERT INTO users(account,account_type,password_hash,role,kyc_status) VALUES($1,'email','x','buyer','verified') RETURNING id`, account).Scan(&owner.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO workbench_workspaces(account_id,slug,display_name) VALUES($1,'personal','Personal') RETURNING id`, owner.AccountID).Scan(&owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	return New(pool), pool, owner
}

func requireLimit(t *testing.T, err error, code string) {
	t.Helper()
	var got *LimitError
	if !errors.As(err, &got) || got.Code != code || got.NextAction == "" {
		t.Fatalf("error=%v, want %s with next_action", err, code)
	}
}

func TestConcurrentAdmissionNeverOversellsWithoutRedis(t *testing.T) {
	m, pool, owner := quotaFixture(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO workbench_quota_policies(account_id,workspace_id,user_concurrent_runs,workspace_concurrent_runs) VALUES($1,$2,1,1)`, owner.AccountID, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, e := m.AdmitRun(ctx, owner, fmt.Sprintf("parallel-%d", i), time.Now())
			errs <- e
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	var admitted, rejected int
	for err := range errs {
		if err == nil {
			admitted++
		} else {
			rejected++
			requireLimit(t, err, "quota_user_concurrent_runs")
		}
	}
	if admitted != 1 || rejected != 1 {
		t.Fatalf("admitted=%d rejected=%d", admitted, rejected)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM workbench_run_reservations WHERE state='active'`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active=%d err=%v", active, err)
	}
}

func TestUsageDebitIsIdempotentAndResetsAcrossMonths(t *testing.T) {
	m, pool, owner := quotaFixture(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO workbench_quota_policies(account_id,workspace_id,monthly_tokens,monthly_compute_millis) VALUES($1,$2,10,100)`, owner.AccountID, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	jan := time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC)
	if ok, err := m.ChargeUsage(ctx, owner, "run", "event-1", 8, 50, 7, jan); err != nil || !ok {
		t.Fatalf("first=%v,%v", ok, err)
	}
	if ok, err := m.ChargeUsage(ctx, owner, "run", "event-1", 8, 50, 7, jan); err != nil || ok {
		t.Fatalf("duplicate=%v,%v", ok, err)
	}
	_, err = m.ChargeUsage(ctx, owner, "run", "event-2", 3, 0, 0, jan)
	requireLimit(t, err, "quota_monthly_tokens")
	feb := jan.Add(2 * time.Hour)
	if ok, err := m.ChargeUsage(ctx, owner, "run", "event-2", 3, 0, 1, feb); err != nil || !ok {
		t.Fatalf("next month=%v,%v", ok, err)
	}
	var janTokens, febTokens int64
	if err := pool.QueryRow(ctx, `SELECT tokens FROM workbench_monthly_ledgers WHERE workspace_id=$1 AND month_start='2026-01-01'`, owner.WorkspaceID).Scan(&janTokens); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT tokens FROM workbench_monthly_ledgers WHERE workspace_id=$1 AND month_start='2026-02-01'`, owner.WorkspaceID).Scan(&febTokens); err != nil {
		t.Fatal(err)
	}
	if janTokens != 8 || febTokens != 3 {
		t.Fatalf("jan=%d feb=%d", janTokens, febTokens)
	}
}

func TestTerminalReleaseBillsWallOnceForFailedAndCanceledRuns(t *testing.T) {
	m, pool, owner := quotaFixture(t)
	ctx := context.Background()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i, status := range []string{"failed", "canceled"} {
		id := fmt.Sprintf("terminal-%d", i)
		if _, err := m.AdmitRun(ctx, owner, id, start); err != nil {
			t.Fatal(err)
		}
		if err := m.CompleteRun(ctx, owner, id, status, start.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := m.CompleteRun(ctx, owner, id, status, start.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	var compute int64
	if err := pool.QueryRow(ctx, `SELECT compute_millis FROM workbench_monthly_ledgers WHERE workspace_id=$1 AND month_start='2026-03-01'`, owner.WorkspaceID).Scan(&compute); err != nil {
		t.Fatal(err)
	}
	if compute != 2000 {
		t.Fatalf("compute=%d, want 2000", compute)
	}
}
