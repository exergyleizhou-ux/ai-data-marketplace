// Package compute manages C2D (Compute-to-Data) algorithms and jobs.
// Sellers submit algorithms; buyers submit compute jobs that run those
// algorithms against purchased datasets. Results carry tamper-proof
// attestations (Ed25519 signatures over input→output hashes).
package compute

import "errors"

// Algo is a registered compute algorithm.
type Algo struct {
	ID             string `json:"id"`
	SellerID       string `json:"seller_id"`
	Name           string `json:"name"`
	Runtime        string `json:"runtime"` // "docker" | "wasm" | "tee"
	Image          string `json:"image"`
	ImageDigest    string `json:"image_digest"` // sha256:...
	Version        int    `json:"version"`
	SourceRef      string `json:"source_ref,omitempty"` // URL to source
	Entrypoint     string `json:"entrypoint"`
	OutputKind     string `json:"output_kind"` // "model" | "metrics" | "report" | "bytes"
	ParamsSchema   string `json:"params_schema"` // JSON schema for runtime params
	CurrentVersion bool   `json:"current_version"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

// Job is one compute run: a buyer applies an algorithm to a purchased dataset.
type Job struct {
	ID          string `json:"id"`
	AlgorithmID string `json:"algorithm_id"`
	BuyerID     string `json:"buyer_id"`
	DatasetID   string `json:"dataset_id"`
	Params      string `json:"params,omitempty"` // JSON, validated against ParamsSchema
	Status      string `json:"status"` // "pending" | "running" | "done" | "failed"
	OutputKind  string `json:"output_kind"`
	OutputBytes int64  `json:"output_bytes"`
	Error       string `json:"error,omitempty"`
	// Attestation chain
	AttestInputHash  string `json:"attest_input_hash,omitempty"` // SHA-256 of input manifest
	AttestOutputHash string `json:"attest_output_hash,omitempty"` // SHA-256 of output
	AttestSignature  string `json:"attest_signature,omitempty"` // Ed25519 sig over input+output
	AttestSignedAt   string `json:"attest_signed_at,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// Result carries the output of a completed compute job.
type Result struct {
	Job       Job    `json:"job"`
	OutputURL string `json:"output_url,omitempty"` // S3 pre-signed URL (if big)
	Output    string `json:"output,omitempty"` // inline output (if small)
}

// ── Validation ────────────────────────────────────────────

var (
	ErrNameRequired       = errors.New("name is required")
	ErrImageRequired      = errors.New("image is required")
	ErrDigestRequired     = errors.New("image_digest is required")
	ErrOutputKindInvalid  = errors.New("output_kind must be one of: model, metrics, report, bytes")
	ErrRuntimeInvalid     = errors.New("runtime must be one of: docker, wasm, tee")
)

var validOutputKinds = map[string]bool{
	"model": true, "metrics": true, "report": true, "bytes": true,
}

var validRuntimes = map[string]bool{
	"docker": true, "wasm": true, "tee": true,
}

// ValidateAlgo checks that an algorithm registration payload is well-formed.
func ValidateAlgo(a *Algo) error {
	if a.Name == "" {
		return ErrNameRequired
	}
	if a.Image == "" {
		return ErrImageRequired
	}
	if a.ImageDigest == "" {
		return ErrDigestRequired
	}
	if a.Runtime == "" {
		a.Runtime = "docker"
	}
	if !validRuntimes[a.Runtime] {
		return ErrRuntimeInvalid
	}
	if a.OutputKind == "" {
		a.OutputKind = "model"
	}
	if !validOutputKinds[a.OutputKind] {
		return ErrOutputKindInvalid
	}
	if a.Version < 1 {
		a.Version = 1
	}
	return nil
}

// DefaultParamsSchema returns the minimal params schema for a given output kind.
func DefaultParamsSchema(kind string) string {
	switch kind {
	case "model":
		return `{"n_estimators":{"type":"integer","default":100},"learning_rate":{"type":"number","default":0.1}}`
	case "metrics":
		return `{"metric":{"type":"string","default":"accuracy"}}`
	default:
		return `{}`
	}
}
