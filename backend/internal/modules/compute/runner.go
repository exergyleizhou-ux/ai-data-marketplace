// Package compute — C2D Docker runner.
//
// Executes compute jobs in Docker containers with strict isolation:
//   --network none         No internet access.
//   --read-only            Root FS is read-only.
//   -v /data:ro            Dataset mount, read-only.
//   -v /out                Output directory, writable.
//
// Images are pulled by pinned digest (sha256:…), never :latest.
// On completion, output.json is read, hashed, Ed25519-attested, and
// dispatched to a persistent storage directory.
package compute

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunRequest bundles everything the runner needs.
type RunRequest struct {
	Job         Job
	Algo        Algo
	DatasetPath string             // absolute path
	OutputDir   string             // writable scratch (caller cleans)
	Params      map[string]any
	SigningKey  ed25519.PrivateKey
	StorageDir  string             // where to persist final attested result
	Timeout     time.Duration      // default 5 min
}

// RunResult is a completed run.
type RunResult struct {
	JobID        string
	OK           bool
	OutputPath   string
	OutputSHA256 string
	Error        string
	InputHash    string
	OutputHash   string
	Signature    string
}

// DockerRunner runs algorithms in containers.
type DockerRunner struct {
	dockerPath string
	NoPull     bool // skip pull in dev/test (image already cached)
	DirectExec bool // for testing: run the binary directly (not via docker)
}

// NewDockerRunner creates a runner.
func NewDockerRunner() *DockerRunner {
	return &DockerRunner{dockerPath: "docker"}
}

// Run executes the algorithm in Docker.
func (r *DockerRunner) Run(ctx context.Context, req RunRequest) RunResult {
	if req.Timeout <= 0 {
		req.Timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	res := RunResult{JobID: req.Job.ID}

	// 1. Build and hash the input manifest.
	manifest := map[string]any{
		"job_id":      req.Job.ID,
		"algorithm":   req.Algo.Name + "@" + req.Algo.ImageDigest,
		"dataset":     req.Job.DatasetID,
		"params":      req.Params,
		"output_kind": req.Algo.OutputKind,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	manifestBytes, _ := json.Marshal(manifest)
	h := sha256.Sum256(manifestBytes)
	res.InputHash = fmt.Sprintf("%x", h[:])

	// 2. Write params to the output dir (passed as stdin to container).
	inputJSON, _ := json.Marshal(map[string]any{
		"dataset_path": "/data",
		"params":       req.Params,
	})
	if err := os.WriteFile(filepath.Join(req.OutputDir, "input.json"), inputJSON, 0644); err != nil {
		res.Error = fmt.Sprintf("write input: %v", err)
		return res
	}

	// 3. Pull image by pinned digest (skip if NoPull).
	imageRef := req.Algo.Image + "@" + req.Algo.ImageDigest
	if !r.NoPull {
		if err := r.pull(ctx, imageRef); err != nil {
			res.Error = fmt.Sprintf("pull: %v", err)
			return res
		}
	}

	// 4. Run container.
	outputPath := filepath.Join(req.OutputDir, "output.json")
	if err := r.run(ctx, imageRef, req.DatasetPath, req.OutputDir); err != nil {
		if data, rerr := os.ReadFile(outputPath); rerr == nil && len(data) > 0 {
			res.OutputPath = outputPath
			res.OutputSHA256 = hashHex(data)
			res.OutputHash = hashHex(data)
		}
		res.Error = fmt.Sprintf("run: %v", err)
		return res
	}

	// 5. Read output.
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		res.Error = fmt.Sprintf("read output: %v", err)
		return res
	}
	res.OutputPath = outputPath
	res.OutputSHA256 = hashHex(outputData)
	res.OutputHash = hashHex(outputData)

	// 6. Attest: sign SHA-256(inputHash || outputHash).
	payload := append([]byte(res.InputHash), []byte(res.OutputHash)...)
	payloadHash := sha256.Sum256(payload)
	sig := ed25519.Sign(req.SigningKey, payloadHash[:])
	res.Signature = fmt.Sprintf("%x", sig)
	res.OK = true

	// 7. Dispatch to storage.
	if req.StorageDir != "" {
		permPath := filepath.Join(req.StorageDir, "c2d-results", req.Job.ID, "result.json")
		os.MkdirAll(filepath.Dir(permPath), 0755)
		permData, _ := json.MarshalIndent(map[string]any{
			"job_id":       req.Job.ID,
			"algorithm":    req.Algo.Name,
			"image_digest": req.Algo.ImageDigest,
			"input_hash":   res.InputHash,
			"output_hash":  res.OutputHash,
			"signature":    res.Signature,
			"output":       json.RawMessage(outputData),
		}, "", "  ")
		os.WriteFile(permPath, permData, 0644)
	}

	return res
}

// pull fetches the pinned image.
func (r *DockerRunner) pull(ctx context.Context, imageRef string) error {
	cmd := exec.CommandContext(ctx, r.dockerPath, "pull", imageRef)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// run starts the container with isolation constraints.
// When DirectExec is true, runs the binary directly instead of via docker.
func (r *DockerRunner) run(ctx context.Context, imageRef, datasetPath, outputDir string) error {
	inputPath := filepath.Join(outputDir, "input.json")
	inputData, _ := os.ReadFile(inputPath)

	if r.DirectExec {
		absData, _ := filepath.Abs(datasetPath)
		absOut, _ := filepath.Abs(outputDir)

		cmd := exec.CommandContext(ctx, r.dockerPath)
		cmd.Stdin = bytes.NewReader(inputData)
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		cmd.Env = append(os.Environ(),
			"C2D_DATA_DIR="+absData,
			"C2D_OUT_DIR="+absOut,
		)
		err := cmd.Run()
		if stdoutBuf.Len() > 0 {
			os.WriteFile(filepath.Join(outputDir, "output.json"), stdoutBuf.Bytes(), 0644)
		}
		if err != nil {
			errMsg := strings.TrimSpace(stderrBuf.String())
			if errMsg == "" { errMsg = err.Error() }
			if len(errMsg) > 500 { errMsg = errMsg[:497] + "..." }
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	}

	absData, _ := filepath.Abs(datasetPath)
	absOut, _ := filepath.Abs(outputDir)

	args := []string{
		"run",
		"--rm",
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64M",
		"-v", absData + ":/data:ro",
		"-v", absOut + ":/out:rw",
		"-i",
		imageRef,
	}

	if _, err := exec.LookPath(r.dockerPath); err != nil {
		return fmt.Errorf("docker not found — install docker or set COMPUTE_RUNNER=local")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	inputPath = filepath.Join(outputDir, "input.json")
	inputData, _ = os.ReadFile(inputPath)
	cmd.Stdin = bytes.NewReader(inputData)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		if len(errMsg) > 500 {
			errMsg = errMsg[:497] + "..."
		}
		// If container produced stdout, save it as output.json
		if stdoutBuf.Len() > 0 {
			outputPath := filepath.Join(outputDir, "output.json")
			os.WriteFile(outputPath, stdoutBuf.Bytes(), 0644)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Container succeeded — save stdout as output.json
	outputPath := filepath.Join(outputDir, "output.json")
	if err := os.WriteFile(outputPath, stdoutBuf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// ── Helpers ────────────────────────────────────────────────

func hashHex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}
