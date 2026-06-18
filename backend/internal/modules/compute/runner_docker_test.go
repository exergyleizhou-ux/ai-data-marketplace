package compute

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// TestDockerRunArgs_SecurityFlags pins the L1 isolation flags. These ARE the
// security boundary, so they are asserted independently of any Docker daemon.
func TestDockerRunArgs_SecurityFlags(t *testing.T) {
	req := RunRequest{
		Algorithm:      Algorithm{Image: "reg/vo-logreg:1", ImageDigest: "sha256:abc123", OutputKind: OutputModel},
		MaxRuntimeSecs: 60,
	}
	args := dockerRunArgs(req, DefaultDockerResources, "/stage/data", "/stage/out", "/stage/params.json")
	joined := strings.Join(args, " ")

	for _, must := range []string{
		"--network=none",
		"--read-only",
		"--security-opt=no-new-privileges",
		"--cap-drop=ALL",
		"--user=65534:65534",
		"--pids-limit=128",
		"--memory=512m",
		"--cpus=1",
		"/stage/data:/data:ro",
		"/stage/out:/out",
		"/stage/params.json:/params.json:ro",
	} {
		if !strings.Contains(joined, must) {
			t.Errorf("docker args missing %q\nargs: %s", must, joined)
		}
	}
	// tmpfs for /tmp must be present (rootfs is read-only).
	if !strings.Contains(joined, "--tmpfs=/tmp:") {
		t.Errorf("missing writable /tmp tmpfs: %s", joined)
	}
	// A `--` separator must precede the image so a crafted image string can't be
	// parsed as a docker flag; the digest-pinned image is the LAST arg.
	if args[len(args)-2] != "--" {
		t.Errorf("missing -- separator before image: %v", args)
	}
	if got := args[len(args)-1]; got != "reg/vo-logreg@sha256:abc123" {
		t.Errorf("last arg = %q, want digest-pinned image", got)
	}
	// The data mount must be read-only.
	if !strings.Contains(joined, "/data:ro") {
		t.Errorf("data mount is not read-only: %s", joined)
	}
}

func TestImageRef_PinsDigest(t *testing.T) {
	cases := []struct {
		image, digest, want string
	}{
		{"reg/vo-logreg:1", "sha256:abc", "reg/vo-logreg@sha256:abc"},
		{"reg/vo-logreg", "sha256:abc", "reg/vo-logreg@sha256:abc"},
		{"reg/vo-logreg:1", "", "reg/vo-logreg:1"},                              // no digest -> bare image (tag kept)
		{"reg:5000/vo-logreg:1", "sha256:def", "reg:5000/vo-logreg@sha256:def"}, // registry port preserved
		{"reg:5000/vo-logreg", "sha256:def", "reg:5000/vo-logreg@sha256:def"},
	}
	for _, c := range cases {
		got := imageRef(Algorithm{Image: c.image, ImageDigest: c.digest})
		if got != c.want {
			t.Errorf("imageRef(%q,%q) = %q, want %q", c.image, c.digest, got, c.want)
		}
	}
}

func TestDockerRunner_Contract(t *testing.T) {
	r := NewDockerRunner(DockerResources{})
	if r.Kind() != "docker" {
		t.Errorf("kind = %q", r.Kind())
	}
	if !r.NeedsStagedData() {
		t.Error("docker runner must need staged data")
	}
	if MockRunner.NeedsStagedData(MockRunner{}) {
		t.Error("mock runner must NOT need staged data")
	}
}

// TestStageData round-trips a dataset object through local storage into a staged
// file the sandbox can mount, then cleans up.
func TestStageData(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	ctx := context.Background()
	want := []byte("f1,f2,label\n1,2,0\n3,4,1\n")
	if _, err := uploadOutput(ctx, store, "datasets/x/data.csv", want); err != nil {
		t.Fatalf("seed object: %v", err)
	}

	dir, cleanup, err := stageData(ctx, store, "datasets/x/data.csv")
	if err != nil {
		t.Fatalf("stageData: %v", err)
	}
	got, err := os.ReadFile(dir + "/input.csv")
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("staged bytes mismatch:\n got %q\nwant %q", got, want)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove staging dir %s", dir)
	}
}

func TestDockerRunArgs_RuntimeSelectable(t *testing.T) {
	req := RunRequest{Algorithm: Algorithm{Image: "img", ImageDigest: "sha256:x", OutputKind: OutputModel}}

	// Default runtime → no --runtime flag.
	def := strings.Join(dockerRunArgs(req, DefaultDockerResources, "/d", "/o", "/p.json"), " ")
	if strings.Contains(def, "--runtime") {
		t.Errorf("default runtime should not pass --runtime: %s", def)
	}

	// gVisor (runsc) → --runtime=runsc present, and the run subcommand stays first.
	res := DefaultDockerResources
	res.Runtime = "runsc"
	gv := dockerRunArgs(req, res, "/d", "/o", "/p.json")
	if gv[0] != "run" {
		t.Fatalf("first arg = %q, want run", gv[0])
	}
	if !strings.Contains(strings.Join(gv, " "), "--runtime=runsc") {
		t.Errorf("gVisor runtime missing --runtime=runsc: %v", gv)
	}
	// Hardening flags must still be present alongside the runtime.
	if !strings.Contains(strings.Join(gv, " "), "--network=none") {
		t.Errorf("runtime selection dropped --network=none: %v", gv)
	}
}

func TestNewDockerRunner_PreservesRuntimeWithDefaults(t *testing.T) {
	// Setting only Runtime must keep the default caps (not wipe them).
	r := NewDockerRunner(DockerResources{Runtime: "runsc"}).(dockerRunner)
	if r.res.Runtime != "runsc" || r.res.Memory != DefaultDockerResources.Memory || r.res.PidsLimit != DefaultDockerResources.PidsLimit {
		t.Fatalf("withDefaults dropped fields: %+v", r.res)
	}
}
