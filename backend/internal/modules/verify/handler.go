package verify

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ repo Repository }

// Register mounts the public certificate verification endpoint.
func Register(rg *gin.RouterGroup, repo Repository) {
	h := &handler{repo: repo}
	rg.GET("/verify/:cert_id", h.lookup)
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
	httpx.OK(c, gin.H{
		"cert_id":       ci.CertID,
		"resource_type": ci.ResourceType,
		"resource_id":   ci.ResourceID,
		"registered_at": ci.CreatedAt,
		"status":        "registered",
		"verifiable":    true,
		"statement_zh":  "该存证凭证已登记，可在平台对应资源详情页查看完整凭证。",
		"statement_en":  "This certificate is registered. Visit the resource detail page for the full certificate.",
	})
}
