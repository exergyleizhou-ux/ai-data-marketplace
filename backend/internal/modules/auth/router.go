package auth

import "github.com/gin-gonic/gin"

// Register mounts the auth routes under the given /api/v1 group. The server
// composes modules by calling each module's Register — this is the only entry
// point the server knows about (modular-monolith boundary).
func Register(rg *gin.RouterGroup, svc *Service, tm *TokenManager) {
	h := &handler{svc: svc}

	pub := rg.Group("/auth")
	pub.POST("/register", h.register)
	pub.POST("/login", h.login)
	pub.POST("/refresh", h.refresh)

	// Protected routes require a valid access token.
	authed := rg.Group("")
	authed.Use(Middleware(tm))
	authed.GET("/users/me", h.me)
}
