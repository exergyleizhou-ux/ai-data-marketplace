package workbench

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRuntimeIngestRejectsOversizedJSONBeforePersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/runs/:id", runtimeAuth(strings.Repeat("s", 32)), runtimeHandler{}.createRun)
	body := `{"owner":{"account_id":"a","workspace_id":"w"},"run":{"request":"` + strings.Repeat("x", (1<<20)+1) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/runs/run-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 32))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("oversized runtime JSON status = %d, want 400", w.Code)
	}
}

func TestRuntimeCredentialRejectsWrongSecretWithoutEchoingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/runs/:id", runtimeAuth(strings.Repeat("s", 32)), runtimeHandler{}.createRun)
	req := httptest.NewRequest(http.MethodPost, "/runs/run-1", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer attacker-secret-that-must-not-be-logged")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "attacker-secret") {
		t.Fatalf("credential rejection status/body = %d %q", w.Code, w.Body.String())
	}
}
