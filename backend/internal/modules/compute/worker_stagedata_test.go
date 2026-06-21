package compute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The sandbox container runs as a non-root uid (65534, see runner_docker.go
// --user). On a real Linux Docker host (unlike Docker Desktop, which masks host
// uid perms) it can only traverse the mounted /data dir if that dir grants
// other-execute, and read the staged CSV if the file grants other-read. A 0700
// MkdirTemp dir made /data unreadable to the sandbox → "Permission denied" →
// algorithm_error. This pins the perms that keep C2D working on real hosts.
func TestStageData_ReadableBySandboxUID(t *testing.T) {
	dir, cleanup, err := stageData(context.Background(), blobStore{blob: []byte("a,b\n1,2\n")}, "k")
	if err != nil {
		t.Fatalf("stageData: %v", err)
	}
	defer cleanup()

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm&0o055 != 0o055 {
		t.Fatalf("staged data dir perms = %#o, want group+other r-x so the sandbox uid can traverse /data", perm)
	}

	fi, err := os.Stat(filepath.Join(dir, "input.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o044 != 0o044 {
		t.Fatalf("staged input.csv perms = %#o, want group+other readable for the sandbox uid", perm)
	}
}
