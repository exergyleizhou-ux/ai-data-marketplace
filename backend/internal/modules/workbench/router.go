package workbench

import (
	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/modules/auth"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
	"time"
)

func Register(rg *gin.RouterGroup, s *Service, tm *auth.TokenManager, l ratelimit.Limiter) {
	h := handler{s}
	g := rg.Group("/workbench")
	g.Use(auth.Middleware(tm))
	g.POST("/token", middleware.RateLimit(l, middleware.RateLimitConfig{Name: "workbench_token", Limit: 30, Window: time.Minute}), h.token)
}
