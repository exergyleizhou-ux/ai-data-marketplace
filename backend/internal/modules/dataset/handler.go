package dataset

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

type datasetRequest struct {
	Title               string             `json:"title"`
	Description         string             `json:"description"`
	DataType            string             `json:"data_type"`
	Domain              string             `json:"domain"`
	LicenseType         string             `json:"license_type"`
	SuggestedPriceCents *int64             `json:"suggested_price_cents"`
	SourceDeclaration   *SourceDeclaration `json:"source_declaration"`
}

func (r datasetRequest) toInput() CreateInput {
	return CreateInput{
		Title:               r.Title,
		Description:         r.Description,
		DataType:            r.DataType,
		Domain:              r.Domain,
		LicenseType:         r.LicenseType,
		SuggestedPriceCents: r.SuggestedPriceCents,
		SourceDeclaration:   r.SourceDeclaration,
	}
}

func (h *handler) create(c *gin.Context) {
	var req datasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	d, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), req.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) update(c *gin.Context) {
	var req datasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	d, err := h.svc.Update(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) list(c *gin.Context) {
	atoi := func(q string) int64 { n, _ := strconv.ParseInt(c.Query(q), 10, 64); return n }
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.List(c.Request.Context(), ListFilter{
		Keyword:       c.Query("q"),
		DataType:      c.Query("data_type"),
		LicenseType:   c.Query("license_type"),
		Domain:        c.Query("domain"),
		MinPriceCents: atoi("min_price"),
		MaxPriceCents: atoi("max_price"),
		Sort:          c.Query("sort"),
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Dataset{} // serialize empty result as [] not null
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) preview(c *gin.Context) {
	res, err := h.svc.Preview(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) get(c *gin.Context) {
	d, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) listMine(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	list, err := h.svc.ListMine(c.Request.Context(), httpx.UserID(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if list == nil {
		list = []Dataset{}
	}
	httpx.OK(c, gin.H{"items": list})
}

func (h *handler) signSource(c *gin.Context) {
	d, err := h.svc.SignSource(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

type initUploadRequest struct {
	Filename string `json:"filename"`
}

func (h *handler) initUpload(c *gin.Context) {
	var req initUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Filename == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("filename is required"))
		return
	}
	sess, err := h.svc.InitUpload(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Filename)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, sess)
}

func (h *handler) uploadPart(c *gin.Context) {
	uploadID := c.Query("upload_id")
	partNumber, err := strconv.Atoi(c.Query("part_number"))
	if uploadID == "" || err != nil || partNumber < 1 {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("upload_id and part_number (>=1) are required"))
		return
	}
	n, err := h.svc.UploadPart(c.Request.Context(), httpx.UserID(c), uploadID, partNumber, c.Request.Body)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"part_number": partNumber, "bytes": n})
}

func (h *handler) completeUpload(c *gin.Context) {
	uploadID := c.Query("upload_id")
	if uploadID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("upload_id is required"))
		return
	}
	d, err := h.svc.CompleteUpload(c.Request.Context(), httpx.UserID(c), uploadID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) uploadStatus(c *gin.Context) {
	uploadID := c.Query("upload_id")
	if uploadID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("upload_id is required"))
		return
	}
	st, status, err := h.svc.UploadStatus(c.Request.Context(), httpx.UserID(c), uploadID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"upload": st, "dataset_status": status})
}

type reviewRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
}

func (h *handler) adminList(c *gin.Context) {
	status := c.Query("status")
	if status == "" {
		status = StatusReviewing
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.AdminListByStatus(c.Request.Context(), status, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Dataset{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) review(c *gin.Context) {
	var req reviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	d, err := h.svc.Review(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Approve, req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

type delistRequest struct {
	Reason string `json:"reason"`
}

func (h *handler) delist(c *gin.Context) {
	var req delistRequest
	_ = c.ShouldBindJSON(&req)
	d, err := h.svc.Delist(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not the dataset owner"))
	case errors.Is(err, ErrNotVerified):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("seller must complete real-name verification"))
	case errors.Is(err, ErrNotEditable):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("dataset can only be edited in draft or rejected state"))
	case errors.Is(err, ErrAlreadySigned):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("source declaration already signed"))
	case errors.Is(err, ErrNotSigned):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("source declaration must be signed first"))
	case errors.Is(err, ErrUploadForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("upload does not belong to caller"))
	case errors.Is(err, ErrStorageUnavailable):
		httpx.Fail(c, httpx.ErrInternal.WithMessage("storage not configured"))
	case errors.Is(err, ErrNotReviewable):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("dataset is not awaiting review"))
	case errors.Is(err, ErrNotPublished):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("dataset is not published"))
	case errors.Is(err, ErrBadTransition):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("illegal status transition"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
