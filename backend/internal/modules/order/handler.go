package order

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

type createRequest struct {
	DatasetID   string `json:"dataset_id"`
	LicenseType string `json:"license_type"`
}

func (h *handler) create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DatasetID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("dataset_id and license_type are required"))
		return
	}
	o, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), req.DatasetID, req.LicenseType)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

func (h *handler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	var (
		items []Order
		err   error
	)
	if c.Query("role") == "seller" {
		items, err = h.svc.ListSales(c.Request.Context(), httpx.UserID(c), limit, offset)
	} else {
		items, err = h.svc.ListMine(c.Request.Context(), httpx.UserID(c), limit, offset)
	}
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) get(c *gin.Context) {
	o, err := h.svc.Get(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

func (h *handler) confirmDelivery(c *gin.Context) {
	o, err := h.svc.ConfirmDelivery(c.Request.Context(), httpx.UserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

type disputeRequest struct {
	Reason string `json:"reason"`
}

func (h *handler) dispute(c *gin.Context) {
	var req disputeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("reason is required"))
		return
	}
	o, err := h.svc.Dispute(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not a party to this order"))
	case errors.Is(err, ErrNotVerified):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("buyer must complete real-name verification"))
	case errors.Is(err, ErrNotPurchasable):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("dataset is not available for purchase"))
	case errors.Is(err, ErrSelfPurchase):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("cannot buy your own dataset"))
	case errors.Is(err, ErrDuplicateOrder):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("an active order for this dataset already exists"))
	case errors.Is(err, ErrBadTransition):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("illegal order status transition"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
