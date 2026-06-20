package compute

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ScreenResult is the product of an ad-hoc, self-serve verification screen.
type ScreenResult struct {
	CertID          string         `json:"certificate_id"`
	OutputSHA256    string         `json:"output_sha256"`
	OutputBytes     int64          `json:"output_bytes"`
	AlgorithmDigest string         `json:"algorithm_digest"`
	Report          map[string]any `json:"report"`
}

// ScreenAdhoc runs the trusted PaperGuard integrity screen on an uploaded dataset
// directly — the thin, self-serve path behind the Verify API. It reuses the SAME
// hardened sandbox runner, output gate, storage, and public certificate machinery
// as the marketplace flow, but WITHOUT any dataset/offer/entitlement/job: a paying
// API caller gets back a quality report + a re-hash-verifiable certificate.
//
// Requires the execution engine (runner+store) and a cert registrar to be wired
// (production: COMPUTE_RUNNER=docker). Returns an error otherwise.
func (s *Service) ScreenAdhoc(ctx context.Context, ownerID string, data []byte) (ScreenResult, error) {
	if s.runner == nil || s.store == nil {
		return ScreenResult{}, fmt.Errorf("screening engine not configured")
	}
	if len(data) == 0 {
		return ScreenResult{}, fmt.Errorf("empty dataset")
	}

	algo, err := s.trustedScreener(ctx)
	if err != nil {
		return ScreenResult{}, err
	}

	// Stage the uploaded bytes as a read-only dataset dir for the sandbox.
	dir, err := os.MkdirTemp("", "verify-adhoc-")
	if err != nil {
		return ScreenResult{}, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "input.csv"), data, 0o644); err != nil {
		return ScreenResult{}, err
	}
	_ = os.Chmod(dir, 0o755) // readable by the container's uid 65534

	id := uuid.NewString()
	res, err := s.runner.Run(ctx, RunRequest{
		Job:            Job{ID: id, DatasetID: id},
		Algorithm:      algo,
		DataPath:       dir,
		MaxOutputBytes: 1 << 20,
		MaxRuntimeSecs: 120,
	})
	if err != nil {
		return ScreenResult{}, fmt.Errorf("screen failed: %w", err)
	}

	// Same anti-exfil output gate as the marketplace path.
	if v := GateOutput(res.OutputKind, res.Output, policyForKind(res.OutputKind, 1<<20)); v != nil {
		return ScreenResult{}, fmt.Errorf("output rejected by gate: %s", v.Reason)
	}

	key := fmt.Sprintf("verify-adhoc/%s/output.bin", id)
	size, err := uploadOutput(ctx, s.store, key, res.Output)
	if err != nil {
		return ScreenResult{}, fmt.Errorf("store output: %w", err)
	}

	sum := sha256.Sum256(res.Output)
	sha := hex.EncodeToString(sum[:])
	certID := jobCertificateID(id, sha)
	if s.certReg != nil {
		if err := s.certReg.Register(ctx, certID, "compute_result", id); err != nil {
			return ScreenResult{}, fmt.Errorf("register cert: %w", err)
		}
	}

	return ScreenResult{
		CertID:          certID,
		OutputSHA256:    sha,
		OutputBytes:     size,
		AlgorithmDigest: algo.ImageDigest,
		Report:          extractReport(res.Output),
	}, nil
}

// trustedScreener resolves the trusted PaperGuard integrity-screen algorithm.
func (s *Service) trustedScreener(ctx context.Context) (Algorithm, error) {
	algos, err := s.repo.ListApprovedAlgorithms(ctx)
	if err != nil {
		return Algorithm{}, err
	}
	for _, a := range algos {
		if !a.Trusted {
			continue
		}
		if strings.Contains(strings.ToLower(a.Name), "paperguard") ||
			strings.Contains(strings.ToLower(a.Image), "paperguard") {
			return a, nil
		}
	}
	return Algorithm{}, fmt.Errorf("no trusted integrity-screen algorithm registered")
}

// extractReport returns the aggregate report from the output: metrics.json from a
// zip-of-json, or the JSON object itself. Best-effort (nil if neither parses) —
// the certificate already binds the exact bytes regardless.
func extractReport(output []byte) map[string]any {
	if zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output))); err == nil {
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), "metrics.json") {
				rc, err := f.Open()
				if err != nil {
					continue
				}
				b, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
				rc.Close()
				var m map[string]any
				if json.Unmarshal(b, &m) == nil {
					return m
				}
			}
		}
	}
	var m map[string]any
	if json.Unmarshal(output, &m) == nil {
		return m
	}
	return nil
}
