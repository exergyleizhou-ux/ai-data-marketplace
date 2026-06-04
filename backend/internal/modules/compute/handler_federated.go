package compute

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type federatedSubmitRequest struct {
	AlgorithmID     string         `json:"algorithm_id"`
	DatasetIDs      []string       `json:"dataset_ids"`
	Params          map[string]any `json:"params"`
	DPEpsilon       *float64       `json:"dp_epsilon"`
	MinParticipants int            `json:"min_participants"` // 0 ⇒ all datasets; else tolerate dropouts down to this many
}

func (h *handler) submitFederated(c *gin.Context) {
	var req federatedSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.DatasetIDs) < 2 {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("dataset_ids (>=2) are required"))
		return
	}
	fed, err := h.svc.SubmitFederatedJob(c.Request.Context(), httpx.UserID(c), FederatedSubmitInput{
		AlgorithmID: req.AlgorithmID, DatasetIDs: req.DatasetIDs, Params: req.Params, DPEpsilon: req.DPEpsilon,
		MinParticipants: req.MinParticipants,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, fed)
}

func (h *handler) listMyFederated(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.ListFederatedJobs(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []FederatedJob{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) getFederated(c *gin.Context) {
	fed, subs, err := h.svc.GetFederatedJob(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if subs == nil {
		subs = []Job{}
	}
	httpx.OK(c, gin.H{"federated_job": fed, "sub_jobs": subs})
}

func (h *handler) federatedOutput(c *gin.Context) {
	rc, size, fed, err := h.svc.OpenFederatedOutput(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Disposition", "attachment; filename=\"federated-model-"+fed.ID+".json\"")
	c.Header("X-Output-Kind", "fedmodel")
	c.DataFromReader(http.StatusOK, size, "application/json", rc, nil)
}

func (h *handler) federatedCertificate(c *gin.Context) {
	cert, err := h.svc.GetFederatedCertificate(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, cert)
}
