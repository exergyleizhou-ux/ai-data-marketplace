package auth

import (
	"errors"
	"strconv"

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

type updateProfileRequest struct {
	Role string `json:"role"`
}

func (h *handler) updateProfile(c *gin.Context) {
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	user, err := h.svc.UpdateRole(c.Request.Context(), httpx.UserID(c), req.Role)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, user)
}

type submitKYCRequest struct {
	Type         string   `json:"type"`
	RealName     string   `json:"real_name"`
	CompanyName  string   `json:"company_name"`
	IDNo         string   `json:"id_no"`
	MaterialURLs []string `json:"material_urls"`
}

func (h *handler) submitKYC(c *gin.Context) {
	var req submitKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	rec, err := h.svc.SubmitKYC(c.Request.Context(), httpx.UserID(c),
		req.Type, req.RealName, req.CompanyName, req.IDNo, req.MaterialURLs)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rec)
}

func (h *handler) getKYC(c *gin.Context) {
	rec, err := h.svc.GetKYC(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rec)
}

type reviewKYCRequest struct {
	KYCID   string `json:"kyc_id"`
	Approve bool   `json:"approve"`
}

func (h *handler) adminListKYC(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	items, err := h.svc.ListPendingKYC(c.Request.Context(), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []KYCRecord{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *handler) adminReviewKYC(c *gin.Context) {
	var req reviewKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.KYCID == "" {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	rec, err := h.svc.ReviewKYC(c.Request.Context(), req.KYCID, req.Approve, httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rec)
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
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrKYCNotFound):
		httpx.Fail(c, httpx.ErrNotFound)
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
