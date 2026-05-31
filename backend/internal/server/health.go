package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleHealthz is a pure liveness probe: the process is up and serving.
func (s *Server) handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleReadyz is a readiness probe. In later PRs it pings Postgres and Redis
// and returns 503 when a dependency is down. For PR-01 there are no
// dependencies wired yet, so it always reports ready.
func (s *Server) handleReadyz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ready", "env": s.cfg.Env})
}
