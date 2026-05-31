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
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
