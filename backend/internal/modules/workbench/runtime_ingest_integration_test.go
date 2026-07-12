package workbench

import (
	"bytes"
	"context"
	"encoding/json"
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
