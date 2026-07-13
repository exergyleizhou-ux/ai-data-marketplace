package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformdb "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func runtimeRepo(t *testing.T) (*Repository, *pgxpool.Pool, Owner, Owner) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	if err := platformdb.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	ctx := context.Background()
	lock, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_lock(9283746)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = lock.Exec(context.Background(), `SELECT pg_advisory_unlock(9283746)`); lock.Release() })
	_, _ = p.Exec(ctx, `TRUNCATE workbench_usage,workbench_artifacts,workbench_approvals,workbench_events,workbench_runs,workbench_workspaces CASCADE`)
	seed := func(n int) Owner {
		var uid, wid string
		err := p.QueryRow(ctx, `INSERT INTO users(account,account_type,password_hash,role,kyc_status) VALUES($1,'email','x','buyer','verified') RETURNING id`, fmt.Sprintf("runtime-%d-%d@example.test", n, time.Now().UnixNano())).Scan(&uid)
		if err != nil {
			t.Fatal(err)
		}
		err = p.QueryRow(ctx, `INSERT INTO workbench_workspaces(account_id,slug,display_name) VALUES($1,'personal','Personal') RETURNING id`, uid).Scan(&wid)
		if err != nil {
			t.Fatal(err)
		}
		return Owner{uid, wid}
	}
	return NewRepository(p), p, seed(1), seed(2)
}

func seedRun(t *testing.T, p *pgxpool.Pool, o Owner, id string) {
	t.Helper()
	_, err := p.Exec(context.Background(), `INSERT INTO workbench_runs(id,account_id,workspace_id,profile,status,request) VALUES($1,$2,$3,'code','running','{}')`, id, o.AccountID, o.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Exec(context.Background(), `INSERT INTO workbench_run_reservations(run_id,account_id,workspace_id,started_at,last_heartbeat_at,expires_at) VALUES($1,$2,$3,now(),now(),now()+interval '2 minutes') ON CONFLICT DO NOTHING`, id, o.AccountID, o.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeRepositoryOwnerCASAndIdempotency(t *testing.T) {
	r, p, a, b := runtimeRepo(t)
	ctx := context.Background()
	seedRun(t, p, a, "run-a")
	if _, err := r.GetRun(ctx, b, "run-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get=%v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, e := r.UpdateRunCAS(ctx, a, "run-a", 1, "completed", json.RawMessage(`{"ok":true}`))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	var success, conflict int
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Errorf("CAS: %v", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS success=%d conflict=%d", success, conflict)
	}
	e := Event{ID: "evt-1", RunID: "run-a", AccountID: a.AccountID, WorkspaceID: a.WorkspaceID, Seq: 1, Type: "status", Payload: json.RawMessage(`{}`), CreatedAt: time.Now()}
	if ok, err := r.AppendEvent(ctx, e); err != nil || !ok {
		t.Fatalf("append=%v,%v", ok, err)
	}
	if ok, err := r.AppendEvent(ctx, e); err != nil || ok {
		t.Fatalf("duplicate=%v,%v", ok, err)
	}
	u := Usage{RunID: "run-a", EventID: "evt-1", AccountID: a.AccountID, WorkspaceID: a.WorkspaceID, Provider: "openai", Model: "model", InputTokens: 3, CostMicrounits: 9, OccurredAt: time.Now()}
	if ok, err := r.RecordUsage(ctx, u); err != nil || !ok {
		t.Fatalf("usage=%v,%v", ok, err)
	}
	if ok, err := r.RecordUsage(ctx, u); err != nil || ok {
		t.Fatalf("duplicate usage=%v,%v", ok, err)
	}
}

func TestRuntimeApprovalOwnerExpiryAndArtifact(t *testing.T) {
	r, p, a, b := runtimeRepo(t)
	ctx := context.Background()
	seedRun(t, p, a, "run-a")
	_, err := p.Exec(ctx, `INSERT INTO workbench_approvals(approval_id,run_id,tool_call_id,account_id,workspace_id,owner,risk_level,reason,effects,args_hash,expires_at) VALUES('ap-1','run-a','tc-1',$1::uuid,$2::uuid,$1::text,'high','writes file','{"writes":true}',repeat('a',64),now()+interval '1 hour')`, a.AccountID, a.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.DecideApproval(ctx, b, "ap-1", b.AccountID, "approved", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner decide=%v", err)
	}
	v, err := r.DecideApproval(ctx, a, "ap-1", a.AccountID, "approved", 1)
	if err != nil || v.Decision == nil || *v.Decision != "approved" {
		t.Fatalf("decision=%+v %v", v, err)
	}
	_, err = p.Exec(ctx, `INSERT INTO workbench_artifacts(id,run_id,account_id,workspace_id,name,kind,object_key,sha256,size_bytes) VALUES('art-1','run-a',$1,$2,'report','report','managed/key',repeat('b',64),7)`, a.AccountID, a.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	arts, err := r.ListArtifacts(ctx, a, "run-a")
	if err != nil || len(arts) != 1 {
		t.Fatalf("artifacts=%d %v", len(arts), err)
	}
	if arts, err := r.ListArtifacts(ctx, b, "run-a"); err != nil || len(arts) != 0 {
		t.Fatalf("cross artifacts=%d %v", len(arts), err)
	}
}

func TestArtifactObjectKeyRejectsPaths(t *testing.T) {
	o := Owner{"account", "workspace"}
	if got, err := ArtifactObjectKey(o, "run", "artifact"); err != nil || got != "workbench/account/workspace/run/artifact" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := ArtifactObjectKey(o, "../run", "artifact"); err == nil {
		t.Fatal("expected path rejection")
	}
}
