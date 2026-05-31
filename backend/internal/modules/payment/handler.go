package payment

import (
	"errors"
	"io"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

type createRequest struct {
	OrderID string `json:"order_id"`
}

func (h *handler) create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.OrderID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("order_id is required"))
		return
	}
	info, err := h.svc.CreatePayment(c.Request.Context(), httpx.UserID(c), req.OrderID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, info)
}

// webhook is the provider callback. It is unauthenticated (the provider has no
// JWT) — authenticity comes from signature verification inside the service.
func (h *handler) webhook(c *gin.Context) {
	payload, _ := io.ReadAll(c.Request.Body)
	// Stripe signs with the Stripe-Signature header; the mock uses X-Signature.
	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		signature = c.GetHeader("X-Signature")
	}
	if err := h.svc.HandleCallback(c.Request.Context(), c.Param("channel"), payload, signature); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"received": true})
}

// devMarkPaid simulates a paid callback (sandbox/dev only).
func (h *handler) devMarkPaid(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.OrderID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("order_id is required"))
		return
	}
	if err := h.svc.DevMarkPaid(c.Request.Context(), req.OrderID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"order_id": req.OrderID, "status": "paid"})
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not the buyer of this order"))
	case errors.Is(err, ErrOrderNotPayable):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("order is not in a payable state"))
	case errors.Is(err, ErrNotConfirmed):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("order is not confirmed"))
	case errors.Is(err, ErrInvalidSignature):
		httpx.Fail(c, httpx.ErrUnauthorized.WithMessage("invalid callback signature"))
	default:
		slog.Error("payment handler error", "path", c.FullPath(), "err", err)
		httpx.Fail(c, httpx.ErrInternal)
	}
}
