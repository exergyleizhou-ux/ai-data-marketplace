// Package api tests the embedded OpenAPI 3.0 specification.
package api

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPI_IsValidYAML parses openapi.yaml as YAML and asserts it has the
// mandatory OpenAPI 3.0 top-level keys.
func TestOpenAPI_IsValidYAML(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(Spec, &doc); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}
	for _, key := range []string{"openapi", "info", "paths"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("openapi.yaml missing required top-level key %q", key)
		}
	}
	ver, _ := doc["openapi"].(string)
	if !strings.HasPrefix(ver, "3.") {
		t.Errorf("openapi version should be 3.x, got %q", ver)
	}
	paths, _ := doc["paths"].(map[string]any)
	if len(paths) < 20 {
		t.Errorf("expected ≥20 path entries, got %d — spec may be incomplete", len(paths))
	}
	t.Logf("openapi.yaml: version=%s, paths=%d, valid YAML", ver, len(paths))
}

// TestDocsHandler_ServesYAML verifies the embedded spec is non-empty.
func TestDocsHandler_ServesYAML(t *testing.T) {
	if len(Spec) == 0 {
		t.Fatal("embedded openapi.yaml is empty")
	}
	if !strings.Contains(string(Spec), "openapi:") {
		t.Fatal("openapi.yaml does not contain 'openapi:' header")
	}
	t.Logf("embedded openapi.yaml: %d bytes", len(Spec))
}
