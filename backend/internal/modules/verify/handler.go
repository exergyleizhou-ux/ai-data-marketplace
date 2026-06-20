package verify

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct{ repo Repository }

// Register mounts the public certificate verification endpoint. It is rate
// limited because it is unauthenticated and the cert_id space is small — without
// a throttle it is a brute-force enumeration oracle.
func Register(rg *gin.RouterGroup, repo Repository, limiter ratelimit.Limiter) {
	h := &handler{repo: repo}
	rg.GET("/verify/:cert_id",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "verify_lookup", Limit: 30, Window: time.Minute}),
		h.lookup)
	// Embeddable SVG status badge (the viral loop): a green "verified" badge for a
	// registered cert, grey otherwise. Always 200 so an embedded <img> degrades.
	rg.GET("/verify/:cert_id/badge.svg",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "verify_badge", Limit: 60, Window: time.Minute}),
		h.badge)
}

func (h *handler) lookup(c *gin.Context) {
	ci, err := h.repo.FindByCertID(c.Request.Context(), c.Param("cert_id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, httpx.ErrNotFound.WithMessage("certificate not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	// Do NOT echo the internal resource_id (dataset/job UUID): an anonymous caller
	// who brute-forces or recomputes a cert_id would otherwise harvest internal
	// identifiers + their registration timestamps. The holder of a cert verifies
	// it from the resource detail page, which is properly authorized.
	httpx.OK(c, gin.H{
		"cert_id":       ci.CertID,
		"resource_type": ci.ResourceType,
		"registered_at": ci.CreatedAt,
		"status":        "registered",
		"verifiable":    true,
		"statement_zh":  "该存证凭证已登记，可在平台对应资源详情页查看完整凭证。",
		"statement_en":  "This certificate is registered. Visit the resource detail page for the full certificate.",
	})
}
