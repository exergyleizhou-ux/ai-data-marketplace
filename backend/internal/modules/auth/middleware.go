package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// Middleware verifies the Bearer access token and injects the caller's id and
// role into the context (read via httpx.UserID / httpx.UserRole). Requests
// without a valid token are rejected with 401 before reaching the handler.
//
// It lives in the auth module (not platform) because it depends on the
// TokenManager; the shared identity contract lives in httpx so other modules
// read identity without importing auth.
func Middleware(tm *TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.Fail(c, httpx.ErrUnauthorized)
			c.Abort()
			return
		}
		claims, err := tm.Parse(token, tokenTypeAccess)
		if err != nil {
			httpx.Fail(c, httpx.ErrUnauthorized)
			c.Abort()
			return
		}
		c.Set(httpx.AuthUserIDKey, claims.UserID)
		c.Set(httpx.AuthRoleKey, claims.Role)
		c.Next()
	}
}
