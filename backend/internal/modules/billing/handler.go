package billing

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// Register mounts the billing endpoints: a JWT-authed checkout-session creator
// and the public, signature-verified Stripe webhook.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}
	g := rg.Group("/billing")
	g.POST("/checkout", authMW, h.checkout)
	g.POST("/stripe/webhook", h.webhook) // public; verified by Stripe signature
}

type checkoutRequest struct {
	PriceID string `json:"price_id"`
}

func (h *handler) checkout(c *gin.Context) {
	if !h.svc.Enabled() {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("paid plans are not configured yet"))
		return
	}
	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PriceID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("price_id is required"))
		return
	}
	url, err := h.svc.CheckoutURL(c.Request.Context(), httpx.UserID(c), req.PriceID)
	if err != nil {
		if errors.Is(err, ErrUnknownPrice) {
			httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("unknown price"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, gin.H{"checkout_url": url})
}

func (h *handler) webhook(c *gin.Context) {
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	switch err := h.svc.HandleWebhook(payload, c.GetHeader("Stripe-Signature")); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"received": true})
	case errors.Is(err, ErrDisabled):
		c.JSON(http.StatusOK, gin.H{"received": true, "billing": "disabled"}) // ack so Stripe stops retrying
	case errors.Is(err, ErrInvalidSignature):
		httpx.Fail(c, httpx.ErrUnauthorized.WithMessage("invalid signature"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
