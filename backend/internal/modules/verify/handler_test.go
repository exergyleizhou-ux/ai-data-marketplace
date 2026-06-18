package verify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type fakeRepo struct {
	ci  *CertInfo
	err error
}

func (f fakeRepo) FindByCertID(_ context.Context, _ string) (*CertInfo, error) {
	return f.ci, f.err
}

func (f fakeRepo) Register(_ context.Context, _, _, _ string) error { return nil }

func newEngine(repo Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	Register(e.Group("/api/v1"), repo, ratelimit.NewInMemory())
	return e
}

// TestVerifyDoesNotLeakResourceID: the public lookup must confirm a cert is
// registered (type + timestamp) WITHOUT echoing the internal resource_id (the
// dataset/job UUID) — otherwise the endpoint is an enumeration oracle that
// harvests internal identifiers.
func TestVerifyDoesNotLeakResourceID(t *testing.T) {
	repo := fakeRepo{ci: &CertInfo{
		CertID: "VO-ABCDEF012345", ResourceType: "dataset",
		ResourceID: "secret-internal-uuid-9999", CreatedAt: "2026-06-18T00:00:00Z",
	}}
	w := httptest.NewRecorder()
	newEngine(repo).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/verify/VO-ABCDEF012345", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "secret-internal-uuid-9999") {
		t.Fatalf("response leaked internal resource_id: %s", body)
	}
	// The non-sensitive registration facts are still returned.
	if !strings.Contains(body, "dataset") || !strings.Contains(body, "registered") {
		t.Fatalf("response should confirm registration type/status: %s", body)
	}
}
