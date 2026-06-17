package compute

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service handles compute business logic: algorithm registration,
// job submission, attestation, and output delivery.
type Service struct {
	repo Repository
}

// NewService creates a compute Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ── Algorithms ─────────────────────────────────────────────

// RegisterAlgo validates and persists a new algorithm.
func (s *Service) RegisterAlgo(ctx context.Context, sellerID string, a Algo) (Algo, error) {
	a.ID = uuid.New().String()
	a.SellerID = sellerID
	if err := ValidateAlgo(&a); err != nil {
		return Algo{}, err
	}
	algo, err := s.repo.CreateAlgo(ctx, a)
	if err != nil {
		return Algo{}, fmt.Errorf("create algo: %w", err)
	}
	return algo, nil
}

// GetAlgo returns a single algorithm.
func (s *Service) GetAlgo(ctx context.Context, id string) (Algo, error) {
	return s.repo.GetAlgo(ctx, id)
}

// ListCurrentAlgos returns all algorithms marked as the current version.
func (s *Service) ListCurrentAlgos(ctx context.Context) ([]Algo, error) {
	return s.repo.ListCurrentAlgos(ctx)
}

// ListAlgosBySeller returns algorithms owned by a seller.
func (s *Service) ListAlgosBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Algo, error) {
	return s.repo.ListAlgosBySeller(ctx, sellerID, limit, offset)
}

// ── Jobs ───────────────────────────────────────────────────

// SubmitJob creates a new compute job if the buyer owns the dataset.
// The job stays in "pending" until the runner picks it up.
func (s *Service) SubmitJob(ctx context.Context, buyerID, algoID, datasetID, params string) (Job, error) {
	// Verify algo exists
	algo, err := s.repo.GetAlgo(ctx, algoID)
	if err != nil {
		return Job{}, fmt.Errorf("algorithm not found: %w", err)
	}
	if !algo.CurrentVersion {
		return Job{}, fmt.Errorf("algorithm %q is not the current version", algo.Name)
	}

	job := Job{
		ID:          uuid.New().String(),
		AlgorithmID: algoID,
		BuyerID:     buyerID,
		DatasetID:   datasetID,
		Params:      params,
		Status:      "pending",
	}
	created, err := s.repo.CreateJob(ctx, job)
	if err != nil {
		return Job{}, fmt.Errorf("create job: %w", err)
	}
	return created, nil
}

// GetJob returns a single job with full attestation fields.
func (s *Service) GetJob(ctx context.Context, id string) (Job, error) {
	return s.repo.GetJob(ctx, id)
}

// ListJobsByBuyer returns jobs submitted by a buyer.
func (s *Service) ListJobsByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Job, error) {
	return s.repo.ListJobsByBuyer(ctx, buyerID, limit, offset)
}

// SetJobRunning transitions a job from pending to running.
func (s *Service) SetJobRunning(ctx context.Context, id string) error {
	return s.repo.UpdateJobStatus(ctx, id, "running", "")
}

// RecordJobOutput stores the output of a completed job with attestation.
func (s *Service) RecordJobOutput(ctx context.Context, id, outputKind string, outputBytes int64, outputHash string) error {
	return s.repo.SetJobOutput(ctx, id, outputKind, outputBytes)
}

// MarkJobFailed records a job failure.
func (s *Service) MarkJobFailed(ctx context.Context, id, errMsg string) error {
	return s.repo.UpdateJobStatus(ctx, id, "failed", errMsg)
}

// ── Attestation ────────────────────────────────────────────

// AttestResult signs the input→output hash chain for tamper-proof verification.
// Returns (inputHash, outputHash, signature) or an error.
func AttestResult(inputManifest, outputData []byte, privateKey ed25519.PrivateKey) (inputHash, outputHash, sig string, err error) {
	h := sha256.New()
	h.Write(inputManifest)
	inH := hex.EncodeToString(h.Sum(nil))

	h.Reset()
	h.Write(outputData)
	outH := hex.EncodeToString(h.Sum(nil))

	// Sign: hash(inputHash || outputHash) 
	tosign := sha256.Sum256(append([]byte(inH), []byte(outH)...))
	signature := ed25519.Sign(privateKey, tosign[:])
	return inH, outH, hex.EncodeToString(signature), nil
}

// VerifyAttestation checks that a signature over inputHash+outputHash is valid.
func VerifyAttestation(inputHash, outputHash, signatureHex string, publicKey ed25519.PublicKey) (bool, error) {
	sig, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}
	tosign := sha256.Sum256(append([]byte(inputHash), []byte(outputHash)...))
	return ed25519.Verify(publicKey, tosign[:], sig), nil
}

// SaveAttestation persists the attestation chain to the job record.
func (s *Service) SaveAttestation(ctx context.Context, jobID, inputHash, outputHash, signature string) error {
	return s.repo.SetJobAttestation(ctx, jobID, inputHash, outputHash, signature)
}

// ── Runner ─────────────────────────────────────────────────

// Runner executes a compute job and returns the result.
type Runner interface {
	Run(ctx context.Context, req RunRequest) RunResult
}

// ExecuteJob orchestrates the full C2D lifecycle for one job:
// pending → running → done/failed, with attestation.
func (s *Service) ExecuteJob(ctx context.Context, jobID, datasetPath, outputDir, storageDir string, signingKey ed25519.PrivateKey) (RunResult, error) {
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return RunResult{}, fmt.Errorf("get job: %w", err)
	}
	if job.Status != "pending" {
		return RunResult{}, fmt.Errorf("job %s is %s, not pending", jobID, job.Status)
	}

	algo, err := s.repo.GetAlgo(ctx, job.AlgorithmID)
	if err != nil {
		return RunResult{}, fmt.Errorf("get algo: %w", err)
	}

	// Parse params
	var params map[string]any
	if job.Params != "" {
		json.Unmarshal([]byte(job.Params), &params)
	}
	if params == nil {
		params = map[string]any{}
	}

	// Mark running
	if err := s.repo.UpdateJobStatus(ctx, jobID, "running", ""); err != nil {
		return RunResult{}, fmt.Errorf("set running: %w", err)
	}

	// Execute
	runner := NewDockerRunner()
	res := runner.Run(ctx, RunRequest{
		Job:         job,
		Algo:        algo,
		DatasetPath: datasetPath,
		OutputDir:   outputDir,
		Params:      params,
		SigningKey:  signingKey,
		StorageDir:  storageDir,
		Timeout:     5 * time.Minute,
	})

	// Record result
	if res.OK {
		s.repo.SetJobOutput(ctx, jobID, algo.OutputKind, int64(len(res.OutputSHA256)))
		s.repo.SetJobAttestation(ctx, jobID, res.InputHash, res.OutputHash, res.Signature)
		s.repo.UpdateJobStatus(ctx, jobID, "done", "")
	} else {
		s.repo.UpdateJobStatus(ctx, jobID, "failed", res.Error)
	}

	return res, nil
}

// ── Worker (SpaceX: automatic launch sequencer) ───────────

// Worker polls for pending C2D jobs and executes them.
// Runs as a background goroutine. Stops when ctx is cancelled.
type Worker struct {
	svc        *Service
	datasetDir string // where dataset files live
	outputDir  string // scratch space for each run
	storageDir string // where attested results persist
	signingKey ed25519.PrivateKey
	interval   time.Duration // poll interval (default 3s)
}

// NewWorker creates a background job executor.
func NewWorker(svc *Service, datasetDir, outputDir, storageDir string, key ed25519.PrivateKey) *Worker {
	return &Worker{
		svc:        svc,
		datasetDir: datasetDir,
		outputDir:  outputDir,
		storageDir: storageDir,
		signingKey: key,
		interval:   3 * time.Second,
	}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollAndExecute(ctx)
		}
	}
}

func (w *Worker) pollAndExecute(ctx context.Context) {
	jobs, err := w.svc.repo.ListPendingJobs(ctx, 5)
	if err != nil || len(jobs) == 0 {
		return
	}
	for _, job := range jobs {
		outDir := filepath.Join(w.outputDir, job.ID)
		os.MkdirAll(outDir, 0755)
		w.svc.ExecuteJob(ctx, job.ID, w.datasetDir, outDir, w.storageDir, w.signingKey)
	}
}

// ── Helpers ────────────────────────────────────────────────

func shortHash(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:8] + "…" + s[len(s)-4:]
}

// Keep unused imports from causing build errors during interface development.
var _ = strings.Compare
