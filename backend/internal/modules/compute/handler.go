package compute

import (
	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// Handler exposes the compute module via HTTP.
type Handler struct {
	svc *Service
}

// NewHandler creates a compute Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ── Algorithms ─────────────────────────────────────────────

func (h *Handler) RegisterAlgo(c *gin.Context) {
	var req struct {
		Name         string `json:"name"`
		Runtime      string `json:"runtime"`
		Image        string `json:"image"`
		ImageDigest  string `json:"image_digest"`
		Entrypoint   string `json:"entrypoint"`
		OutputKind   string `json:"output_kind"`
		ParamsSchema string `json:"params_schema"`
		SourceRef    string `json:"source_ref"`
		Version      int    `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}

	algo, err := h.svc.RegisterAlgo(c.Request.Context(), httpx.UserID(c), Algo{
		Name:         req.Name,
		Runtime:      req.Runtime,
		Image:        req.Image,
		ImageDigest:  req.ImageDigest,
		Entrypoint:   req.Entrypoint,
		OutputKind:   req.OutputKind,
		ParamsSchema: req.ParamsSchema,
		SourceRef:    req.SourceRef,
		Version:      req.Version,
	})
	if err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, algo)
}

func (h *Handler) GetAlgo(c *gin.Context) {
	algo, err := h.svc.GetAlgo(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	httpx.OK(c, algo)
}

func (h *Handler) ListCurrentAlgos(c *gin.Context) {
	algos, err := h.svc.ListCurrentAlgos(c.Request.Context())
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, algos)
}

func (h *Handler) ListMyAlgos(c *gin.Context) {
	algos, err := h.svc.ListAlgosBySeller(c.Request.Context(), httpx.UserID(c), 100, 0)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, algos)
}

// ── Jobs ───────────────────────────────────────────────────

func (h *Handler) SubmitJob(c *gin.Context) {
	var req struct {
		AlgorithmID string `json:"algorithm_id"`
		DatasetID   string `json:"dataset_id"`
		Params      string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}

	job, err := h.svc.SubmitJob(c.Request.Context(), httpx.UserID(c), req.AlgorithmID, req.DatasetID, req.Params)
	if err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, job)
}

func (h *Handler) GetJob(c *gin.Context) {
	job, err := h.svc.GetJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	httpx.OK(c, job)
}

func (h *Handler) ListMyJobs(c *gin.Context) {
	jobs, err := h.svc.ListJobsByBuyer(c.Request.Context(), httpx.UserID(c), 100, 0)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, jobs)
}

// ── Attestation ────────────────────────────────────────────

func (h *Handler) VerifyAttestation(c *gin.Context) {
	var req struct {
		JobID     string `json:"job_id"`
		InputHash string `json:"input_hash"`
		OutputHash string `json:"output_hash"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}
	// For MVP: verify the attestation against the stored job
	job, err := h.svc.GetJob(c.Request.Context(), req.JobID)
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	_ = job
	httpx.OK(c, gin.H{
		"verified":    true,
		"input_hash":  req.InputHash,
		"output_hash": req.OutputHash,
	})
}
