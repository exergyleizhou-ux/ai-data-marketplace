package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"net/http"
	"net/url"
	"strings"
)

// CSRF rejects cross-origin unsafe requests and, when a session exists,
// requires a double-submit token. Login has no session yet, so Origin is the gate.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin != "" {
			u, e := url.Parse(origin)
			if e != nil || !strings.EqualFold(u.Host, c.Request.Host) {
				httpx.Fail(c, httpx.ErrForbidden)
				c.Abort()
				return
			}
		}
		if _, e := c.Cookie("oasis_refresh"); e == nil {
			cookie, _ := c.Cookie("oasis_csrf")
			if cookie == "" || c.GetHeader("X-CSRF-Token") != cookie {
				httpx.Fail(c, httpx.ErrForbidden)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
