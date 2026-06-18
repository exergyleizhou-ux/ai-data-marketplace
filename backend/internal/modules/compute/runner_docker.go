package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DockerResources caps a sandbox container's resources (design §7.1) and selects
// the container runtime — the P2 isolation lever (design §7.2): the default runc
// shares the host kernel, whereas runsc (gVisor) or kata (Kata Containers) give
// a user-space / micro-VM kernel boundary. Swapping Runtime needs no code change.
type DockerResources struct {
	Memory    string // e.g. "512m"
	CPUs      string // e.g. "1"
	PidsLimit int    // e.g. 128
	TmpfsSize string // writable /tmp size, e.g. "64m"
	Runtime   string // "" = default (runc) | "runsc" (gVisor) | "kata" — P2 hardening
}

// DefaultDockerResources are conservative P1 caps (default runtime).
var DefaultDockerResources = DockerResources{Memory: "512m", CPUs: "1", PidsLimit: 128, TmpfsSize: "64m"}

// withDefaults fills empty resource fields from the defaults, preserving any set
// field (e.g. a caller that sets only Runtime keeps the default caps).
func (r DockerResources) withDefaults() DockerResources {
	if r.Memory == "" {
		r.Memory = DefaultDockerResources.Memory
	}
	if r.CPUs == "" {
		r.CPUs = DefaultDockerResources.CPUs
	}
	if r.PidsLimit == 0 {
		r.PidsLimit = DefaultDockerResources.PidsLimit
	}
	if r.TmpfsSize == "" {
		r.TmpfsSize = DefaultDockerResources.TmpfsSize
	}
	return r
}

// dockerRunner runs an algorithm in a hardened `docker run` container: no
// network, read-only rootfs, dropped capabilities, resource caps, the dataset
// mounted read-only at /data, output collected from /out (design §7.1/§18.3).
//
// NOTE: this requires a Docker daemon on the runner host; it is NOT exercised in
// CI (no docker-in-docker) or on the docker-less dev box — the MockRunner is the
// default. The security-critical argument construction is unit-tested
// (dockerRunArgs); selecting it needs COMPUTE_RUNNER=docker + a built, digest-
// pinned algorithm image.
type dockerRunner struct {
	res DockerResources
}

// NewDockerRunner returns a docker-backed Runner with the given resource caps
// (zero value -> DefaultDockerResources).
func NewDockerRunner(res DockerResources) Runner {
	return dockerRunner{res: res.withDefaults()}
}

func (dockerRunner) Kind() string          { return "docker" }
func (dockerRunner) NeedsStagedData() bool { return true }

// imageRef pins the image by digest when the algorithm carries one (design §4:
// trusted algorithms MUST pin a sha256 digest; a mutable :latest tag would void
// the audit). Falls back to the bare image when no digest is set.
func imageRef(a Algorithm) string {
	if strings.HasPrefix(a.ImageDigest, "sha256:") {
		base := a.Image
		if i := strings.IndexByte(base, '@'); i >= 0 {
			base = base[:i]
		}
		if j := strings.LastIndexByte(base, ':'); j >= 0 && !strings.Contains(base[j:], "/") {
			base = base[:j] // strip a tag so we pin purely by digest
		}
		return base + "@" + a.ImageDigest
	}
	return a.Image
}

// dockerRunArgs builds the full `docker run ...` argument vector (everything
// after the "docker" binary). The isolation flags here ARE the L1 security
// boundary, so they are unit-tested independently of any Docker daemon.
func dockerRunArgs(req RunRequest, res DockerResources, dataDir, outDir, paramsFile string) []string {
	args := []string{"run", "--rm"}
	if res.Runtime != "" {
		args = append(args, "--runtime="+res.Runtime) // P2: gVisor (runsc) / Kata kernel boundary (§7.2)
	}
	args = append(args,
		"--network=none",                   // no network: the only exfil path is the gated output
		"--read-only",                      // immutable rootfs
		"--security-opt=no-new-privileges", // no privilege escalation
		"--cap-drop=ALL",                   // drop all Linux capabilities
		"--pids-limit="+strconv.Itoa(res.PidsLimit),
		"--memory="+res.Memory,
		"--cpus="+res.CPUs,
		"--tmpfs=/tmp:rw,size="+res.TmpfsSize+",nodev,nosuid,noexec",
		"--user=65534:65534", // non-root nobody, even if the image declares USER root
		"-v", dataDir+":/data:ro", // dataset, read-only
		"-v", outDir+":/out", // output collection
		"-v", paramsFile+":/params.json:ro", // job params, read-only
		"--", // stop option parsing: a crafted image string can't inject docker flags
		imageRef(req.Algorithm),
	)
	return args
}

// Run stages params, runs the hardened container under a runtime timeout, and
// collects /out/output.bin as the single output object. Algorithm stderr is
// captured as Logs (the worker decides whether/how to surface it — §7.4); it is
// never returned to the caller verbatim here.
func (r dockerRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if req.DataPath == "" {
		return RunResult{}, fmt.Errorf("docker runner requires a staged data path")
	}
	outDir, err := os.MkdirTemp("", "c2d-out-")
	if err != nil {
		return RunResult{}, err
	}
	defer os.RemoveAll(outDir)

	effParams := req.Params
	if effParams == nil {
		effParams = req.Job.Params
	}
	paramsFile, err := writeParams(effParams)
	if err != nil {
		return RunResult{}, err
	}
	defer os.Remove(paramsFile)

	secs := req.MaxRuntimeSecs
	if secs <= 0 {
		secs = 1800
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(secs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, "docker", dockerRunArgs(req, r.res, req.DataPath, outDir, paramsFile)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return RunResult{}, fmt.Errorf("sandbox execution failed: %w", err)
	}

	out, err := os.ReadFile(filepath.Join(outDir, "output.bin"))
	if err != nil {
		return RunResult{}, fmt.Errorf("algorithm produced no output")
	}
	return RunResult{OutputKind: req.Algorithm.OutputKind, Output: out, Logs: stderr.Bytes()}, nil
}

func writeParams(params map[string]any) (string, error) {
	f, err := os.CreateTemp("", "c2d-params-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, _ := json.Marshal(params)
	if _, err := f.Write(b); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
