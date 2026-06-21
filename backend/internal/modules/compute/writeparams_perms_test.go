package compute

import (
	"encoding/json"
	"os"
	"testing"
)

// The job params file is mounted at /params.json:ro into a sandbox container that
// runs as uid 65534. os.CreateTemp makes it 0600 (owned by the runner-host user),
// so on a real Linux Docker host the sandbox uid can't read it — and an algorithm
// that catches the read error (e.g. estimate.py's load_params) silently sees no
// params and falls back to defaults, ignoring the buyer's chosen analysis. Same
// uid-perm class as the staged /data dir. The params file must be world-readable.
func TestWriteParams_ReadableBySandboxUID(t *testing.T) {
	path, err := writeParams(map[string]any{"treatment": "alcohol", "outcome": "quality"})
	if err != nil {
		t.Fatalf("writeParams: %v", err)
	}
	defer os.Remove(path)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o044 != 0o044 {
		t.Fatalf("params file perms = %#o, want group+other readable for the sandbox uid", perm)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m["treatment"] != "alcohol" {
		t.Fatalf("params file content wrong: %s (err=%v)", b, err)
	}
}
