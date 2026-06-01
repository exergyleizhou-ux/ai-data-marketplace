package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

// RateLimitConfig configures one rate-limited route group.
type RateLimitConfig struct {
	Name    string                      // namespace for the key (e.g. "login")
	Limit   int                         // max requests per window
	Window  time.Duration               // window length
	KeyFunc func(c *gin.Context) string // identity within the namespace (e.g. IP)
}

// RateLimit enforces the configured limit using the given limiter. On limiter
// errors it fails open (allows the request) so a Redis outage degrades to
// "no limiting" rather than a full outage.
func RateLimit(limiter ratelimit.Limiter, cfg RateLimitConfig) gin.HandlerFunc {
	keyFunc := cfg.KeyFunc
	if keyFunc == nil {
		keyFunc = KeyByIP
	}
	return func(c *gin.Context) {
		key := "rl:" + cfg.Name + ":" + keyFunc(c)
		res, err := limiter.Allow(c.Request.Context(), key, cfg.Limit, cfg.Window)
		if err != nil {
			c.Next() // fail open
			return
		}
		if !res.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())+1))
			httpx.Fail(c, httpx.ErrRateLimited)
			c.Abort()
			return
		}
		c.Next()
	}
}

// KeyByIP rate-limits per client IP — the default for anonymous endpoints.
func KeyByIP(c *gin.Context) string { return c.ClientIP() }
