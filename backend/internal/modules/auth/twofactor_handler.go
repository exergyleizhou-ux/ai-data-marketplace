package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// --- 2FA TOTP ---

func (h *handler) enroll2FA(c *gin.Context) {
	res, err := h.svc.Enroll2FA(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, res)
}

type twoFACodeReq struct {
	Code string `json:"code"`
}

func (h *handler) verify2FAEnrollment(c *gin.Context) {
	var req twoFACodeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("code is required"))
		return
	}
	if err := h.svc.Verify2FAEnrollment(c.Request.Context(), httpx.UserID(c), req.Code); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *handler) disable2FA(c *gin.Context) {
	var req twoFACodeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("code is required"))
		return
	}
	if err := h.svc.Disable2FA(c.Request.Context(), httpx.UserID(c), req.Code); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

type twoFAChallengeReq struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}

func (h *handler) verify2FAChallenge(c *gin.Context) {
	var req twoFAChallengeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ChallengeToken == "" || req.Code == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("challenge_token and code are required"))
		return
	}
	tokens, user, err := h.svc.Verify2FAChallenge(c.Request.Context(), req.ChallengeToken, req.Code)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"user": user, "tokens": tokens})
}

// --- Password reset ---

type passwordResetRequest struct {
	Account string `json:"account"`
}

func (h *handler) requestPasswordReset(c *gin.Context) {
	var req passwordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Account == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("account is required"))
		return
	}
	// Always return ok — don't leak account existence.
	_ = h.svc.RequestPasswordReset(c.Request.Context(), req.Account)
	httpx.OK(c, gin.H{"ok": true})
}

type passwordResetComplete struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *handler) completePasswordReset(c *gin.Context) {
	var req passwordResetComplete
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || req.NewPassword == "" {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("token and new_password are required"))
		return
	}
	if err := h.svc.CompletePasswordReset(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
