package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestComputeHTTPIntegration drives the full C2D loop over real HTTP handlers
// (gin httptest) against a real Postgres + local object storage:
// ops registers+approves an algorithm → seller enables the offer → buyer lists
// algorithms, purchases, submits a job, polls to released, downloads the EXACT
// output bytes; a non-owner is forbidden.
func TestComputeHTTPIntegration(t *testing.T) {
	dsn := envDSN(t)
	if dsn == "" {
		return
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	repo := NewRepository(pool)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	seller := seedUser(t, pool, "hseller", "seller")
	buyer := seedUser(t, pool, "hbuyer", "buyer")
	dsID := seedDataset(t, pool, seller)

	svc := NewService(repo,
		fakeIdentity{status: kycVerified},
		fakeDatasets{info: DatasetInfo{SellerID: seller, VersionID: "", Published: true}},
		nil,
		WithWorker(NewMockRunner(), store, fakeData{key: "datasets/x"}, 2, 16))
	defer svc.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	Register(api, svc, testAuth(), testOpsGate(), ratelimit.NewInMemory(), true)

	// helper to issue a request as a given user/role and decode data.
	call := func(method, path, user, role string, body any, out any) int {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("X-Test-User", user)
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if out != nil && w.Code == http.StatusOK {
			var env struct {
				Code int             `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope (%s %s): %v body=%s", method, path, err, w.Body.String())
			}
			if err := json.Unmarshal(env.Data, out); err != nil {
				t.Fatalf("decode data (%s %s): %v", method, path, err)
			}
		}
		return w.Code
	}

	// 1. ops registers an algorithm (pending).
	var algo Algorithm
	if code := call(http.MethodPost, "/api/v1/admin/compute/algorithms", "ops-1", "ops",
		registerAlgoRequest{Name: "logreg", Runtime: RuntimeSklearn, Image: "registry/logreg",
			ImageDigest: "sha256:http", SourceRef: "git:http", OutputKind: OutputModel}, &algo); code != 200 {
		t.Fatalf("register algo: code=%d", code)
	}
	if algo.ID == "" || algo.Status != AlgoPending {
		t.Fatalf("registered algo wrong: %+v", algo)
	}

	// 2. ops approves it as trusted.
	var approved Algorithm
	if code := call(http.MethodPost, "/api/v1/admin/compute/algorithms/"+algo.ID+"/review", "ops-1", "ops",
		reviewAlgoRequest{Status: AlgoApproved, Trusted: true}, &approved); code != 200 {
		t.Fatalf("review algo: code=%d", code)
	}
	if approved.Status != AlgoApproved || !approved.Trusted {
		t.Fatalf("approved algo wrong: %+v", approved)
	}

	// A non-ops user cannot register.
	if code := call(http.MethodPost, "/api/v1/admin/compute/algorithms", buyer, "buyer",
		registerAlgoRequest{Name: "x", Runtime: RuntimeSklearn, Image: "x", OutputKind: OutputMetrics}, nil); code != http.StatusForbidden {
		t.Fatalf("buyer register should be 403, got %d", code)
	}

	// 3. seller enables the offer.
	var offer Offer
	if code := call(http.MethodPut, "/api/v1/datasets/"+dsID+"/compute-offer", seller, "seller",
		offerRequest{Enabled: true, TrustLevel: TrustL1, PriceCents: 5000, MaxOutputBytes: 1 << 20}, &offer); code != 200 {
		t.Fatalf("put offer: code=%d", code)
	}
	if !offer.Enabled {
		t.Fatalf("offer not enabled: %+v", offer)
	}

	// 4. buyer lists algorithms for the dataset.
	var algoList struct {
		Items []Algorithm `json:"items"`
	}
	if code := call(http.MethodGet, "/api/v1/compute/algorithms?dataset_id="+dsID, buyer, "buyer", nil, &algoList); code != 200 {
		t.Fatalf("list algos: code=%d", code)
	}
	if len(algoList.Items) == 0 {
		t.Fatal("buyer sees no algorithms")
	}

	// 5. buyer purchases a compute entitlement (dev grant).
	var ent Entitlement
	if code := call(http.MethodPost, "/api/v1/datasets/"+dsID+"/compute/purchase", buyer, "buyer",
		purchaseRequest{Quota: 2}, &ent); code != 200 {
		t.Fatalf("purchase: code=%d", code)
	}
	if ent.ID == "" || ent.JobsQuota != 2 {
		t.Fatalf("entitlement wrong: %+v", ent)
	}

	// 6. buyer submits a job.
	var job Job
	if code := call(http.MethodPost, "/api/v1/compute/jobs", buyer, "buyer",
		submitRequest{DatasetID: dsID, EntitlementID: ent.ID, AlgorithmID: algo.ID}, &job); code != 200 {
		t.Fatalf("submit: code=%d", code)
	}
	if job.ID == "" {
		t.Fatal("no job id")
	}

	// 7. poll until released via the real GET endpoint.
	var released Job
	deadline := time.Now().Add(5 * time.Second)
	for {
		var j Job
		call(http.MethodGet, "/api/v1/compute/jobs/"+job.ID, buyer, "buyer", nil, &j)
		if j.Status == JobReleased {
			released = j
			break
		}
		if JobTerminal(j.Status) {
			t.Fatalf("job terminal %q, want released (err=%q)", j.Status, j.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout; last status %q", j.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if released.OutputKind != OutputModel {
		t.Fatalf("released kind=%q", released.OutputKind)
	}

	// 8. buyer downloads the output bytes.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compute/jobs/"+job.ID+"/output", nil)
	req.Header.Set("X-Test-User", buyer)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatalf("download: code=%d len=%d", w.Code, w.Body.Len())
	}
	if w.Header().Get("X-Output-Kind") != OutputModel {
		t.Fatalf("download missing output-kind header")
	}

	// 9. a non-owner cannot download.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/compute/jobs/"+job.ID+"/output", nil)
	req2.Header.Set("X-Test-User", "intruder")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code == 200 {
		t.Fatal("non-owner download should fail")
	}
}

// testAuth is a stub auth middleware: it injects the user id/role from test
// headers into the httpx context (the real auth middleware does this from a JWT).
func testAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set(httpx.AuthUserIDKey, u)
		}
		if role := c.GetHeader("X-Test-Role"); role != "" {
			c.Set(httpx.AuthRoleKey, role)
		}
		c.Next()
	}
}

// testOpsGate mirrors auth.RequireRole("ops","admin") for the httptest.
func testOpsGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch httpx.UserRole(c) {
		case "ops", "admin":
			c.Next()
		default:
			httpx.Fail(c, httpx.ErrForbidden)
			c.Abort()
		}
	}
}

func envDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB HTTP integration test")
	}
	return dsn
}
