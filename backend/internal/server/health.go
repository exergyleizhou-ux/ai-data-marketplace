package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleHealthz is a pure liveness probe: the process is up and serving.
func (s *Server) handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleReadyz is a readiness probe. It pings dependencies and returns 503 if
// any are down, so load balancers stop routing to a half-broken instance.
func (s *Server) handleReadyz(c *gin.Context) {
	if s.db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "dependency": "postgres"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "env": s.cfg.Env})
}
