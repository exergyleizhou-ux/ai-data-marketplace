package httpx

import "github.com/gin-gonic/gin"

// Auth context keys. The auth middleware sets these after verifying a token;
// any module reads the caller's identity through these helpers — keeping the
// contract in platform (not the auth module) so modules don't depend on auth.
const (
	AuthUserIDKey = "auth_user_id"
	AuthRoleKey   = "auth_role"
)

// UserID returns the authenticated user id, or "" if the request is anonymous.
func UserID(c *gin.Context) string {
	if v, ok := c.Get(AuthUserIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// UserRole returns the authenticated user's role, or "" if anonymous.
func UserRole(c *gin.Context) string {
	if v, ok := c.Get(AuthRoleKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
