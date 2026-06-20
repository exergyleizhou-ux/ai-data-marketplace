package apikey

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// Register mounts the self-serve API-key management endpoints under /api-keys.
// These are JWT-authed (a logged-in account manages its own keys); the keys they
// mint authenticate to the metered Verify API via APIKeyAuth.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc) {
	h := &handler{svc: svc}
	g := rg.Group("/api-keys")
	g.Use(authMW)
	g.POST("", h.issue)
	g.GET("", h.list)
	g.DELETE("/:id", h.revoke)
}

type issueRequest struct {
	Name string `json:"name"`
	Tier string `json:"tier"`
}

// issue mints a new key and returns the plaintext ONCE.
func (h *handler) issue(c *gin.Context) {
	var req issueRequest
	_ = c.ShouldBindJSON(&req) // body is optional
	tier := req.Tier
	if _, ok := Tiers[tier]; !ok {
		tier = "free"
	}
	k, plain, err := h.svc.Issue(c.Request.Context(), httpx.UserID(c), req.Name, tier)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, gin.H{
		"key":    plain, // shown only once
		"id":     k.ID,
		"prefix": k.Prefix,
		"name":   k.Name,
		"tier":   k.Tier,
		"note":   "Store this key now — it is shown only once and cannot be retrieved later.",
	})
}

// list returns the account's keys (metadata only, never the plaintext).
func (h *handler) list(c *gin.Context) {
	keys, err := h.svc.List(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	if keys == nil {
		keys = []APIKey{}
	}
	httpx.OK(c, gin.H{"items": keys})
}

// revoke disables a key the account owns.
func (h *handler) revoke(c *gin.Context) {
	if err := h.svc.Revoke(c.Request.Context(), httpx.UserID(c), c.Param("id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, httpx.ErrNotFound.WithMessage("api key not found"))
			return
		}
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	httpx.OK(c, gin.H{"revoked": true})
}
