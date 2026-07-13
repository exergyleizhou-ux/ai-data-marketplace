package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

const AccessCookie = "oasis_access"
const RefreshCookie = "oasis_refresh"
const CSRFCookie = "oasis_csrf"

func secureCookie(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}
func setBrowserCookies(c *gin.Context, tokens Tokens) {
	secure := secureCookie(c)
	http.SetCookie(c.Writer, &http.Cookie{Name: AccessCookie, Value: tokens.AccessToken, Path: "/", MaxAge: int(tokens.ExpiresIn), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(c.Writer, &http.Cookie{Name: RefreshCookie, Value: tokens.RefreshToken, Path: "/api/v1/auth/session", MaxAge: int((30 * 24 * time.Hour).Seconds()), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	http.SetCookie(c.Writer, &http.Cookie{Name: CSRFCookie, Value: hex.EncodeToString(b), Path: "/", MaxAge: int((30 * 24 * time.Hour).Seconds()), HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode})
}
func clearBrowserCookies(c *gin.Context) {
	for _, p := range []struct{ name, path string }{{AccessCookie, "/"}, {RefreshCookie, "/api/v1/auth/session"}, {CSRFCookie, "/"}} {
		http.SetCookie(c.Writer, &http.Cookie{Name: p.name, Path: p.path, MaxAge: -1, HttpOnly: p.name != CSRFCookie, Secure: secureCookie(c), SameSite: http.SameSiteLaxMode})
	}
}

func (h *handler) sessionLogin(c *gin.Context) {
	var req loginRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	if res.Tokens.AccessToken == "" {
		httpx.OK(c, res)
		return
	}
	setBrowserCookies(c, *res.Tokens)
	res.Tokens = nil
	httpx.OK(c, res)
}
func (h *handler) sessionRegister(c *gin.Context) {
	var req registerRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	res, e := h.svc.Register(c.Request.Context(), req.Account, req.AccountType, req.Password, toAgreements(req.Agreements)...)
	if e != nil {
		fail(c, e)
		return
	}
	setBrowserCookies(c, res.Tokens)
	res.Tokens = Tokens{}
	httpx.OK(c, res)
}
func (h *handler) sessionVerify2FA(c *gin.Context) {
	var req twoFAChallengeReq
	if c.ShouldBindJSON(&req) != nil || req.ChallengeToken == "" || req.Code == "" {
		httpx.Fail(c, httpx.ErrInvalidParam)
		return
	}
	tokens, user, e := h.svc.Verify2FAChallenge(c.Request.Context(), req.ChallengeToken, req.Code)
	if e != nil {
		fail(c, e)
		return
	}
	setBrowserCookies(c, tokens)
	httpx.OK(c, gin.H{"user": user})
}
func (h *handler) sessionRefresh(c *gin.Context) {
	raw, e := c.Cookie(RefreshCookie)
	if e != nil {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	res, e := h.svc.Refresh(c.Request.Context(), raw)
	if e != nil {
		clearBrowserCookies(c)
		fail(c, e)
		return
	}
	setBrowserCookies(c, res.Tokens)
	res.Tokens = Tokens{}
	httpx.OK(c, res)
}
func (h *handler) sessionLogout(c *gin.Context) {
	raw, _ := c.Cookie(RefreshCookie)
	if raw != "" {
		if e := h.svc.Logout(c.Request.Context(), raw); e != nil {
			fail(c, e)
			return
		}
	}
	clearBrowserCookies(c)
	httpx.OK(c, gin.H{"revoked": true})
}
