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

func (h *handler) earnings(c *gin.Context) {
	e, err := h.svc.Earnings(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, e)
}

type reviewRequest struct {
	Score     int    `json:"score"`
	Comment   string `json:"comment"`
	IssueFlag bool   `json:"issue_flag"`
}

func (h *handler) createReview(c *gin.Context) {
	var req reviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	rv, err := h.svc.CreateReview(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Score, req.Comment, req.IssueFlag)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rv)
}

func (h *handler) listReviews(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.ListReviews(c.Request.Context(), c.Param("id"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Review{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) adminTransactions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.AdminTransactions(c.Request.Context(), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Order{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type resolveRequest struct {
	Refund bool   `json:"refund"`
	Note   string `json:"note"`
}

func (h *handler) resolveDispute(c *gin.Context) {
	var req resolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	o, err := h.svc.ResolveDispute(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Refund, req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, o)
}

func (h *handler) adminReconciliation(c *gin.Context) {
	r, err := h.svc.AdminReconciliation(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, r)
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
	case errors.Is(err, ErrReviewExists):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("order already reviewed"))
	case errors.Is(err, ErrNotSettled):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("can only review a settled order"))
	case errors.Is(err, ErrNotDisputed):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("order is not in dispute"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
