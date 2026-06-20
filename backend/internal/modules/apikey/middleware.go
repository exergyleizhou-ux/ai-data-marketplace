package apikey

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// Context keys set by APIKeyAuth so downstream handlers know who is calling.
const (
	ctxAccountID = "apikey_account_id"
	ctxTier      = "apikey_tier"
)

// APIKeyAuth authenticates AND meters an API key on each request — it is the
// billable gate for the Verify API. The key comes from `X-API-Key: sk_live_…` or
// `Authorization: Bearer sk_live_…`. On success the caller's account id + tier are
// stored in the context; 401 for a missing/invalid/revoked key, 429 when the
// monthly quota is spent.
func APIKeyAuth(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := extractKey(c)
		if key == "" {
			httpx.Fail(c, httpx.ErrUnauthorized.WithMessage("missing API key (X-API-Key or Authorization: Bearer sk_…)"))
			c.Abort()
			return
		}
		k, err := svc.Authenticate(c.Request.Context(), key)
		if err != nil {
			switch {
			case errors.Is(err, ErrQuotaExceeded):
				httpx.Fail(c, httpx.ErrRateLimited.WithMessage("monthly quota exceeded for this API key — upgrade your plan"))
			case errors.Is(err, ErrInvalidKey):
				httpx.Fail(c, httpx.ErrUnauthorized.WithMessage("invalid or revoked API key"))
			default:
				httpx.Fail(c, httpx.ErrInternal)
			}
			c.Abort()
			return
		}
		c.Set(ctxAccountID, k.AccountID)
		c.Set(ctxTier, k.Tier)
		c.Next()
	}
}

// extractKey pulls an sk_ key from the X-API-Key header or a Bearer token.
func extractKey(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		t := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if strings.HasPrefix(t, "sk_") {
			return t
		}
	}
	return ""
}

// AccountID returns the api-key caller's account id (set by APIKeyAuth), or "".
func AccountID(c *gin.Context) string {
	if v, ok := c.Get(ctxAccountID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// KeyTier returns the api-key caller's tier (set by APIKeyAuth), or "".
func KeyTier(c *gin.Context) string {
	if v, ok := c.Get(ctxTier); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
