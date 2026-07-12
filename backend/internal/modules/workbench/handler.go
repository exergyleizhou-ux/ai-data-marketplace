package workbench

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ s *Service }

func (h handler) token(c *gin.Context) {
	var q struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := c.ShouldBindJSON(&q); err != nil && !errors.Is(err, io.EOF) {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.s.Issue(c.Request.Context(), httpx.UserID(c), q.WorkspaceID)
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	httpx.OK(c, res)
}
