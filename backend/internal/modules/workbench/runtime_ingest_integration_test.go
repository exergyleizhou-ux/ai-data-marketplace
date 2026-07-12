package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

func TestRuntimeIngestContractPersistsAndIsIdempotent(t *testing.T) {
	repo, pool, owner, _ := runtimeRepo(t)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewManagedService(repo, NewTokenManager("unused", time.Minute), store)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRuntimeIngest(engine.Group("/api/v1"), svc, "runtime-ingest-secret-at-least-32-bytes")
	do := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	if w := do(http.MethodPut, "/api/v1/workbench/runtime/runs/run-ingest", RuntimeRunInput{Owner: owner, Run: Run{Profile: "code", Status: "running", Request: json.RawMessage(`{}`)}}, ""); w.Code != 401 {
		t.Fatalf("anonymous=%d %s", w.Code, w.Body)
	}
	token := "runtime-ingest-secret-at-least-32-bytes"
	create := RuntimeRunInput{Owner: owner, Run: Run{Profile: "code", Status: "running", Request: json.RawMessage(`{}`)}}
	if w := do(http.MethodPut, "/api/v1/workbench/runtime/runs/run-ingest", create, token); w.Code != 201 {
		t.Fatalf("create=%d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPut, "/api/v1/workbench/runtime/runs/run-ingest", create, token); w.Code != 200 {
		t.Fatalf("idempotent create=%d %s", w.Code, w.Body)
	}
	event := RuntimeEventInput{Owner: owner, Event: Event{ID: "evt-ingest", Seq: 1, Type: "started", Payload: json.RawMessage(`{}`)}}
	for i, want := range []bool{true, false} {
		w := do(http.MethodPost, "/api/v1/workbench/runtime/runs/run-ingest/events", event, token)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(fmt.Sprintf(`"recorded":%t`, want))) {
			t.Fatalf("event %d=%d %s", i, w.Code, w.Body)
		}
	}
	usage := RuntimeUsageInput{Owner: owner, Usage: Usage{EventID: "evt-ingest", Provider: "openai", Model: "gpt", InputTokens: 10, CostMicrounits: 42}}
	for i, want := range []bool{true, false} {
		w := do(http.MethodPost, "/api/v1/workbench/runtime/runs/run-ingest/usage", usage, token)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(fmt.Sprintf(`"recorded":%t`, want))) {
			t.Fatalf("usage %d=%d %s", i, w.Code, w.Body)
		}
	}
	artifact := RuntimeArtifactInput{Owner: owner, Artifact: Artifact{ID: "art-ingest", Name: "result.txt", Kind: "evidence", MediaType: "text/plain", ToolCallID: strp("tool-1"), InputRefs: json.RawMessage(`[]`), Provenance: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`)}, Content: []byte("verified output")}
	if w := do(http.MethodPost, "/api/v1/workbench/runtime/runs/run-ingest/artifacts", artifact, token); w.Code != 201 {
		t.Fatalf("artifact=%d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPost, "/api/v1/workbench/runtime/runs/run-ingest/artifacts", artifact, token); w.Code != 200 {
		t.Fatalf("artifact duplicate=%d %s", w.Code, w.Body)
	}
	ap := Approval{ApprovalID: "ap-ingest", ToolCallID: "tool-approval", Owner: owner.AccountID, RiskLevel: "high", Reason: "write", Effects: json.RawMessage(`{"writes":true}`), FileScope: json.RawMessage(`[]`), NetworkTargets: json.RawMessage(`[]`), ExpectedOutputs: json.RawMessage(`[]`), EditableArgs: json.RawMessage(`{}`), ArgsHash: fmt.Sprintf("%064d", 1), ExpiresAt: time.Now().Add(time.Hour)}
	if w := do(http.MethodPost, "/api/v1/workbench/runtime/runs/run-ingest/approvals", RuntimeApprovalInput{Owner: owner, Approval: ap}, token); w.Code != 200 {
		t.Fatalf("approval=%d %s", w.Code, w.Body)
	}
	decided, err := repo.DecideApproval(context.Background(), owner, "ap-ingest", owner.AccountID, "approved", 1)
	if err != nil {
		t.Fatal(err)
	}
	consume := RuntimeConsumeInput{Owner: owner, ArgsHash: ap.ArgsHash, ExecutionID: "exec-ingest", ExpectedVersion: decided.Version}
	w := do(http.MethodPost, "/api/v1/workbench/runtime/approvals/ap-ingest/consume", consume, token)
	if w.Code != 200 {
		t.Fatalf("consume=%d %s", w.Code, w.Body)
	}
	var version int64
	if err := pool.QueryRow(context.Background(), `SELECT version FROM workbench_approvals WHERE approval_id='ap-ingest'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	complete := RuntimeExecutionInput{Owner: owner, ExecutionID: "exec-ingest", State: "executed", ExpectedVersion: version}
	if w := do(http.MethodPost, "/api/v1/workbench/runtime/approvals/ap-ingest/execution", complete, token); w.Code != 200 {
		t.Fatalf("complete=%d %s", w.Code, w.Body)
	}
}
func strp(v string) *string { return &v }

func TestRuntimeIngestFailsClosedWhenUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	RegisterRuntimeIngest(e.Group("/api/v1"), &Service{}, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/workbench/runtime/runs/x", bytes.NewReader([]byte(`{}`)))
	e.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestQuotaMachineContractAdmitsBeforeRunAndReturnsStableErrors(t *testing.T) {
	repo, pool, owner, _ := runtimeRepo(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO workbench_quota_policies(account_id,workspace_id,user_concurrent_runs,workspace_concurrent_runs,monthly_tokens) VALUES($1,$2,1,1,10)`, owner.AccountID, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewManagedService(repo, NewTokenManager("unused", time.Minute), nil)
	gin.SetMode(gin.TestMode)
	e := gin.New()
	secret := "runtime-ingest-secret-at-least-32-bytes"
	RegisterRuntimeIngest(e.Group("/api/v1"), svc, secret)
	do := func(path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}
	admission := quotaAdmissionInput{Owner: owner, StartedAt: time.Now()}
	path := "/api/v1/workbench/runtime/quota/runs/pre-persist/admit"
	if w := do(path, admission); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"run_max_steps"`)) {
		t.Fatalf("admit=%d %s", w.Code, w.Body)
	}
	if w := do(path, admission); w.Code != 200 { // same reservation is idempotent
		t.Fatalf("repeat admit=%d %s", w.Code, w.Body)
	}
	if w := do("/api/v1/workbench/runtime/quota/runs/second/admit", admission); w.Code != 429 || !bytes.Contains(w.Body.Bytes(), []byte(`"code":"quota_user_concurrent_runs"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"next_action":"wait_for_run"`)) {
		t.Fatalf("limit=%d %s", w.Code, w.Body)
	}
	complete := quotaCompletionInput{Owner: owner, Status: "failed", CompletedAt: time.Now().Add(time.Second)}
	if w := do("/api/v1/workbench/runtime/quota/runs/pre-persist/complete", complete); w.Code != 200 {
		t.Fatalf("complete=%d %s", w.Code, w.Body)
	}
	if w := do("/api/v1/workbench/runtime/quota/runs/second/admit", admission); w.Code != 200 {
		t.Fatalf("admit after release=%d %s", w.Code, w.Body)
	}
	usage := RuntimeUsageInput{Owner: owner, Usage: Usage{EventID: "usage-1", InputTokens: 8, CostMicrounits: 3, OccurredAt: time.Now()}}
	usagePath := "/api/v1/workbench/runtime/quota/runs/second/usage"
	if w := do(usagePath, usage); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"recorded":true`)) {
		t.Fatalf("usage=%d %s", w.Code, w.Body)
	}
	if w := do(usagePath, usage); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"recorded":false`)) {
		t.Fatalf("usage duplicate=%d %s", w.Code, w.Body)
	}
}

func TestTerminalCreateDoesNotReserveRunCapacity(t *testing.T) {
	repo, pool, owner, _ := runtimeRepo(t)
	now := time.Now()
	_, created, err := repo.CreateRun(context.Background(), owner, Run{ID: "already-terminal", Profile: "code", Status: "failed", Version: 1, Request: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
	if err != nil || !created {
		t.Fatalf("create=%v,%v", created, err)
	}
	var reservations int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM workbench_run_reservations WHERE run_id='already-terminal'`).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("reservations=%d err=%v", reservations, err)
	}
}

func TestArtifactCommitStateFailureCompensatesMetadataAndObject(t *testing.T) {
	repo, pool, owner, _ := runtimeRepo(t)
	ctx := context.Background()
	now := time.Now()
	_, _, err := repo.CreateRun(ctx, owner, Run{ID: "artifact-compensate", Profile: "code", Status: "running", Version: 1, Request: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE OR REPLACE FUNCTION test_drop_artifact_reservation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN DELETE FROM workbench_artifact_reservations WHERE artifact_id=NEW.id; RETURN NEW; END $$; CREATE TRIGGER test_drop_artifact_reservation AFTER INSERT ON workbench_artifacts FOR EACH ROW EXECUTE FUNCTION test_drop_artifact_reservation()`)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_drop_artifact_reservation ON workbench_artifacts; DROP FUNCTION IF EXISTS test_drop_artifact_reservation()`)
	objects, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewManagedService(repo, NewTokenManager("unused", time.Minute), objects)
	in := RuntimeArtifactInput{Owner: owner, Artifact: Artifact{ID: "commit-fails", RunID: "artifact-compensate", Name: "x", Kind: "evidence", MediaType: "text/plain", Provenance: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`), InputRefs: json.RawMessage(`[]`)}, Content: []byte("must be removed")}
	if _, _, err = svc.StoreArtifact(ctx, in); err == nil {
		t.Fatal("expected quota commit failure")
	}
	if _, err = repo.GetArtifact(ctx, owner, "commit-fails"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("metadata remains: %v", err)
	}
	key, _ := ArtifactObjectKey(owner, "artifact-compensate", "commit-fails")
	if _, _, err = objects.Open(ctx, key); err == nil {
		t.Fatal("object remains after compensated commit failure")
	}
}
