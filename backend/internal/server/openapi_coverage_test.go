package server

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/lei/ai-data-marketplace/backend/api"
	"github.com/lei/ai-data-marketplace/backend/internal/config"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// TestOpenAPI_CoversAllRegisteredRoutes starts the real gin engine, enumerates
// every registered route, and asserts each appears in openapi.yaml.  Gin path
// parameters (:id) are normalised to {id} before comparison.
func TestOpenAPI_CoversAllRegisteredRoutes(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping route-coverage test")
	}

	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	cfg := &config.Config{
		Env:               "test",
		JWTSecret:         "test",
		JWTAccessTTL:      3600,
		JWTRefreshTTL:     7200,
		PIISecret:         "test",
		KYCAutoApprove:    true,
		RedisURL:          "redis://127.0.0.1:1/0",
		StorageDriver:     "local",
		StorageDir:        t.TempDir(),
		CORSAllowOrigin:   "*",
		PaymentProvider:   "mock",
		PaymentMockSecret: "test",
		StripeCurrency:    "usd",
		AppBaseURL:        "https://app.test",
	}
	srv := New(cfg, pool)
	defer srv.Close()

	h := srv.Handler()
	engine, ok := h.(*gin.Engine)
	if !ok {
		t.Fatalf("Server.Handler() is not *gin.Engine, got %T", h)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(api.Spec, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	specPaths, _ := doc["paths"].(map[string]any)

	missing := 0
	for _, r := range engine.Routes() {
		if r.Method == "HEAD" {
			continue
		}
		norm := ginPathToOAPI(r.Path)
		// Strip /api/v1 prefix (the spec's servers[0].url provides it).
		norm = strings.TrimPrefix(norm, "/api/v1")
		// Skip non-API routes.
		if norm == "" || strings.HasPrefix(r.Path, "/docs") || strings.HasPrefix(r.Path, "/metrics") || strings.HasPrefix(r.Path, "/healthz") || strings.HasPrefix(r.Path, "/readyz") || strings.Contains(r.Path, "/ping") {
			continue
		}
		if _, ok := specPaths[norm]; !ok {
			t.Errorf("route %s %s (→ %s) missing from openapi.yaml", r.Method, r.Path, norm)
			missing++
		}
	}

	realSet := map[string]bool{}
	for _, r := range engine.Routes() {
		if r.Method == "HEAD" {
			continue
		}
		norm := ginPathToOAPI(r.Path)
		norm = strings.TrimPrefix(norm, "/api/v1")
		if norm == "" || strings.HasPrefix(r.Path, "/docs") || strings.HasPrefix(r.Path, "/metrics") || strings.HasPrefix(r.Path, "/healthz") || strings.HasPrefix(r.Path, "/readyz") || strings.Contains(r.Path, "/ping") {
			continue
		}
		realSet[norm] = true
	}
	stale := 0
	for sp := range specPaths {
		if !realSet[sp] {
			t.Errorf("spec path %q has no matching real route — stale or typo?", sp)
			stale++
		}
	}
	if stale > 0 {
		t.Fatalf("%d stale spec paths in openapi.yaml — remove or fix them", stale)
	}

	if missing > 0 {
		t.Fatalf("%d routes missing from openapi.yaml — add them", missing)
	}
	t.Logf("all real routes covered in openapi.yaml")
}

func ginPathToOAPI(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if len(s) > 0 && s[0] == ':' {
			segs[i] = "{" + s[1:] + "}"
		}
	}
	return strings.Join(segs, "/")
}
