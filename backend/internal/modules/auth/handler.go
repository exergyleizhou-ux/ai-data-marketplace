package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

type registerRequest struct {
	Account     string `json:"account"`
	AccountType string `json:"account_type"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *handler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.svc.Register(c.Request.Context(), req.Account, req.AccountType, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) me(c *gin.Context) {
	user, err := h.svc.Me(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, user)
}

// fail maps auth sentinel errors onto the uniform httpx error envelope.
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
	case errors.Is(err, ErrAccountExists):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("account already exists"))
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidToken):
		httpx.Fail(c, httpx.ErrUnauthorized.WithMessage(err.Error()))
	case errors.Is(err, ErrUserFrozen):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("user is frozen"))
	case errors.Is(err, ErrUserNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
