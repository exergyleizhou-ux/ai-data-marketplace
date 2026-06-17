package compute

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunnerOutputDispatch(t *testing.T) {
	dir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Create dataset file.
	datasetPath := filepath.Join(dir, "dataset.csv")
	os.WriteFile(datasetPath, []byte("feat1,feat2,target\n1.0,2.0,3.0\n"), 0644)

	// Build mock algo binary.
	algoPath := filepath.Join(dir, "mock-algo")
	buildGoBinary(t, algoPath, `package main
import ("encoding/json";"fmt")
func main() {
	out := map[string]any{"status":"ok","weights":[]float64{1,2,3},"mae":0.05}
	data,_ := json.Marshal(out)
	fmt.Println(string(data))
}`)

	outputDir := filepath.Join(dir, "out")
	os.MkdirAll(outputDir, 0755)
	storageDir := filepath.Join(dir, "storage")

	runner := NewDockerRunner()
	runner.dockerPath = algoPath
	runner.NoPull = true
	runner.DirectExec = true

	req := RunRequest{
		Job:         Job{ID: "job-1", DatasetID: "ds-1", Params: `{"epochs":10}`},
		Algo:        Algo{Name: "mock", Image: "mock", ImageDigest: "sha256:ff", OutputKind: "model"},
		DatasetPath: datasetPath,
		OutputDir:   outputDir,
		Params:      map[string]any{"epochs": float64(10)},
		SigningKey:  priv,
		StorageDir:  storageDir,
	}
	res := runner.Run(context.Background(), req)
	if !res.OK {
		t.Fatalf("run failed: %s", res.Error)
	}
	if res.InputHash == "" || res.OutputHash == "" || res.Signature == "" {
		t.Fatal("empty attestation field")
	}

	// Verify attestation signature.
	payload := append([]byte(res.InputHash), []byte(res.OutputHash)...)
	payloadHash := sha256.Sum256(payload)
	sigBytes, _ := hex.DecodeString(res.Signature)
	if !ed25519.Verify(pub, payloadHash[:], sigBytes) {
		t.Fatal("attestation verification failed")
	}

	// Output dispatched to storage.
	permPath := filepath.Join(storageDir, "c2d-results", "job-1", "result.json")
	if _, err := os.Stat(permPath); err != nil {
		t.Fatalf("output not dispatched: %v", err)
	}
}

func TestRunnerContainerFailure(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	algoPath := filepath.Join(dir, "crash-algo")
	buildGoBinary(t, algoPath, `package main
import ("fmt";"os")
func main() {
	fmt.Println(`+"`"+`{"partial":true}`+"`"+`)
	os.Exit(1)
}`)

	outputDir := filepath.Join(dir, "out2")
	os.MkdirAll(outputDir, 0755)

	runner := NewDockerRunner()
	runner.dockerPath = algoPath
	runner.NoPull = true
	runner.DirectExec = true

	req := RunRequest{
		Job:         Job{ID: "job-2"},
		Algo:        Algo{Name: "crash", Image: "crash", ImageDigest: "sha256:ff", OutputKind: "metrics"},
		DatasetPath: dir,
		OutputDir:   outputDir,
		Params:      map[string]any{},
		SigningKey:  priv,
	}
	res := runner.Run(context.Background(), req)
	if res.OK {
		t.Fatal("expected failure")
	}
	if res.Error == "" {
		t.Fatal("expected error message")
	}
	// Partial output captured.
	if res.OutputHash == "" {
		t.Error("partial output should be captured on failure")
	}
}

func buildGoBinary(t *testing.T, outPath, src string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module mock\n\ngo 1.21\n"), 0644)
	cmd := exec.Command("go", "build", "-o", outPath, ".")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock: %v\n%s", err, stderr.String())
	}
}
