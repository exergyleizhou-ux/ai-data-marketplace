package qa

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/middleware"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/ratelimit"
)

type handler struct{ svc *Service }

// Register mounts QA routes. /datasets/:id/questions is public-read;
// POST /datasets/:id/questions (ask) and POST /questions/:id/answer require auth.
func Register(rg *gin.RouterGroup, svc *Service, authMW gin.HandlerFunc, limiter ratelimit.Limiter) {
	h := &handler{svc: svc}

	rg.GET("/datasets/:id/questions", h.list)

	authed := rg.Group("")
	authed.Use(authMW)
	authed.POST("/datasets/:id/questions",
		middleware.RateLimit(limiter, middleware.RateLimitConfig{Name: "qa_ask", Limit: 10, Window: time.Minute}),
		h.ask)
	authed.POST("/questions/:id/answer", h.answer)
}

func (h *handler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.svc.ListByDataset(c.Request.Context(), c.Param("id"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Question{}
	}
	httpx.OK(c, gin.H{"items": items})
}

type askRequest struct {
	Body string `json:"body"`
}

func (h *handler) ask(c *gin.Context) {
	var req askRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	q, err := h.svc.AskQuestion(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Body)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "ok", "data": q})
}

type answerRequest struct {
	Body string `json:"body"`
}

func (h *handler) answer(c *gin.Context) {
	var req answerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	a, err := h.svc.AnswerQuestion(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.Body)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, a)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrEmptyBody) || errors.Is(err, ErrBodyTooLong):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrQuestionNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	case errors.Is(err, ErrNotSeller):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("only the dataset seller can answer"))
	case errors.Is(err, ErrAlreadyAnswered):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("question already answered"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
