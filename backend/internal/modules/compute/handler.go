package compute

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct {
	svc        *Service
	devEnabled bool // dev-only direct entitlement grant (no real gateway yet)
}

// --- seller: offer ---

type offerRequest struct {
	Enabled        bool     `json:"enabled"`
	AllowCustom    bool     `json:"allow_custom"`
	AllowedAlgoIDs []string `json:"allowed_algorithm_ids"`
	PriceCents     int64    `json:"price_cents"`
	MaxRuntimeSecs int      `json:"max_runtime_secs"`
	MaxOutputBytes int64    `json:"max_output_bytes"`
	MaxOutputFiles int      `json:"max_output_files"`
	DPEpsilon      *float64 `json:"dp_epsilon"`
	DPEpsilonTotal *float64 `json:"dp_epsilon_total"`
	ReturnLogs     bool     `json:"return_logs"`
	ReviewOutput   bool     `json:"review_output"`
	TrustLevel     string   `json:"trust_level"`
}

func (h *handler) putOffer(c *gin.Context) {
	var req offerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	o, err := h.svc.ConfigureOffer(c.Request.Context(), httpx.UserID(c), c.Param("id"), OfferInput{
		Enabled: req.Enabled, AllowCustom: req.AllowCustom, AllowedAlgoIDs: req.AllowedAlgoIDs,
		PriceCents: req.PriceCents, MaxRuntimeSecs: req.MaxRuntimeSecs, MaxOutputBytes: req.MaxOutputBytes,
		MaxOutputFiles: req.MaxOutputFiles, DPEpsilon: req.DPEpsilon, DPEpsilonTotal: req.DPEpsilonTotal,
		ReturnLogs: req.ReturnLogs, ReviewOutput: req.ReviewOutput, TrustLevel: req.TrustLevel,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

func (h *handler) getOffer(c *gin.Context) {
	o, err := h.svc.GetOffer(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

// --- buyer: algorithms / purchase / jobs ---

func (h *handler) listAlgorithms(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("dataset_id is required"))
		return
	}
	items, err := h.svc.ListAlgorithms(c.Request.Context(), datasetID)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Algorithm{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type purchaseRequest struct {
	Quota int `json:"quota"`
}

// purchase grants a compute entitlement. P1: dev-only DIRECT grant (no real
// gateway) — mounted only when devEnabled, mirroring payment's dev mark-paid.
// Real compute purchase goes through order+payment in a follow-up.
func (h *handler) purchase(c *gin.Context) {
	var req purchaseRequest
	_ = c.ShouldBindJSON(&req)
	datasetID := c.Param("id")
	// Validate the offer exists and is enabled before granting.
	offer, err := h.svc.GetOffer(c.Request.Context(), datasetID)
	if err != nil {
		fail(c, err)
		return
	}
	if !offer.Enabled {
		fail(c, ErrOfferDisabled)
		return
	}
	ent, err := h.svc.GrantEntitlement(c.Request.Context(), datasetID, httpx.UserID(c), "", req.Quota)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, ent)
}

// createComputeOrder starts a REAL purchase: create a compute order priced from
// the offer; the buyer then pays it via the existing payment flow, which grants
// the entitlement on success.
func (h *handler) createComputeOrder(c *gin.Context) {
	orderID, err := h.svc.PurchaseViaOrder(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"order_id": orderID})
}

type submitRequest struct {
	DatasetID      string         `json:"dataset_id"`
	EntitlementID  string         `json:"entitlement_id"`
	AlgorithmID    string         `json:"algorithm_id"`
	Params         map[string]any `json:"params"`
	IdempotencyKey string         `json:"idempotency_key"`
}

func (h *handler) submitJob(c *gin.Context) {
	var req submitRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DatasetID == "" || req.EntitlementID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("dataset_id and entitlement_id are required"))
		return
	}
	// Accept an idempotency key from the header too (standard).
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	j, err := h.svc.SubmitJob(c.Request.Context(), httpx.UserID(c), SubmitInput{
		DatasetID: req.DatasetID, EntitlementID: req.EntitlementID, AlgorithmID: req.AlgorithmID,
		Params: req.Params, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) getJob(c *gin.Context) {
	j, err := h.svc.GetJob(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) listMyJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.ListJobs(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Job{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) cancelJob(c *gin.Context) {
	j, err := h.svc.CancelJob(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) listMyEntitlements(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.ListEntitlements(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Entitlement{}
	}
	httpx.OK(c, gin.H{"items": items})
}

// downloadOutput streams a released job's output to its buyer. Mirrors delivery:
// the output bytes leave through this endpoint (or, with a presign-capable
// store, a redirect — a later optimization).
func (h *handler) downloadOutput(c *gin.Context) {
	rc, size, job, err := h.svc.OpenOutput(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Disposition", "attachment; filename=\"compute-output-"+job.ID+".bin\"")
	c.Header("X-Output-Kind", job.OutputKind)
	c.DataFromReader(http.StatusOK, size, "application/octet-stream", rc, nil)
}

// --- ops: algorithm registry + job queue ---

type registerAlgoRequest struct {
	Name         string         `json:"name"`
	Runtime      string         `json:"runtime"`
	Image        string         `json:"image"`
	ImageDigest  string         `json:"image_digest"`
	Version      int            `json:"version"`
	SourceRef    string         `json:"source_ref"`
	Entrypoint   string         `json:"entrypoint"`
	OutputKind   string         `json:"output_kind"`
	ParamsSchema map[string]any `json:"params_schema"`
}

func (h *handler) registerAlgorithm(c *gin.Context) {
	var req registerAlgoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	a, err := h.svc.RegisterAlgorithm(c.Request.Context(), Algorithm{
		Name: req.Name, Runtime: req.Runtime, Image: req.Image, ImageDigest: req.ImageDigest,
		Version: req.Version, SourceRef: req.SourceRef, Entrypoint: req.Entrypoint,
		OutputKind: req.OutputKind, ParamsSchema: req.ParamsSchema,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, a)
}

type reviewAlgoRequest struct {
	Status  string `json:"status"`
	Trusted bool   `json:"trusted"`
}

func (h *handler) reviewAlgorithm(c *gin.Context) {
	var req reviewAlgoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	a, err := h.svc.ReviewAlgorithm(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Status, req.Trusted)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, a)
}

func (h *handler) adminListAlgorithms(c *gin.Context) {
	items, err := h.svc.AdminListAlgorithms(c.Request.Context(), c.Query("status"))
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Algorithm{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) adminListJobs(c *gin.Context) {
	status := c.Query("status")
	if status == "" {
		status = JobOutputReviewing
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.svc.AdminListJobs(c.Request.Context(), status, limit)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Job{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) opsReleaseJob(c *gin.Context) {
	j, err := h.svc.OpsReleaseOutput(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

func (h *handler) opsRejectJob(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	j, err := h.svc.OpsRejectOutput(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, j)
}

// fail maps compute sentinels to the shared httpx envelope (same approach as
// the order module — generic codes, domain-specific messages).
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not permitted"))
	case errors.Is(err, ErrNotVerified):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("buyer must complete real-name verification"))
	case errors.Is(err, ErrOfferDisabled):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("sandbox compute is not enabled for this dataset"))
	case errors.Is(err, ErrAlgoNotAllowed):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("algorithm is not approved/allowed for this dataset"))
	case errors.Is(err, ErrCustomNotAllowed):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("custom algorithms are not allowed on this offer"))
	case errors.Is(err, ErrModelNeedsTrust):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("model output on an L1 offer requires a trusted (audited) algorithm"))
	case errors.Is(err, ErrQuotaExhausted):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("compute entitlement has no remaining job quota"))
	case errors.Is(err, ErrEntitlementState):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("entitlement is not active"))
	case errors.Is(err, ErrDPBudgetExceeded):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("differential-privacy budget exhausted for this dataset"))
	case errors.Is(err, ErrBadTransition):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("illegal job state for this action"))
	case errors.Is(err, ErrSelfPurchase):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("cannot buy compute on your own dataset"))
	case errors.Is(err, ErrPurchasePending):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("a compute order for this dataset is already in progress"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
