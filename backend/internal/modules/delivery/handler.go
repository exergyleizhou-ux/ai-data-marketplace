package delivery

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

type downloadRequest struct {
	LicenseAgreed bool `json:"license_agreed"`
}

// requestDownload: POST /orders/:id/download — buyer accepts the license and
// gets a one-time, short-lived token.
func (h *handler) requestDownload(c *gin.Context) {
	var req downloadRequest
	_ = c.ShouldBindJSON(&req)
	token, expiresAt, err := h.svc.RequestDownload(c.Request.Context(), httpx.UserID(c), c.Param("id"), req.LicenseAgreed)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"download_url": "/api/v1/files/" + token,
		"expires_at":   expiresAt,
	})
}

// download: GET /files/:token — actual byte stream. No JWT; the token is the
// capability. Validity (expiry/quota/IP) is checked in the service.
func (h *handler) download(c *gin.Context) {
	res, err := h.svc.Download(c.Request.Context(), c.Param("token"), c.ClientIP())
	if err != nil {
		fail(c, err)
		return
	}
	if res.RedirectURL != "" {
		// Object storage serves the bytes directly via a short-lived URL.
		c.Redirect(http.StatusFound, res.RedirectURL)
		return
	}
	defer res.Body.Close()
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, res.Body)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("not the buyer of this order"))
	case errors.Is(err, ErrNotPaid):
		httpx.Fail(c, httpx.ErrConflict.WithMessage("order is not paid"))
	case errors.Is(err, ErrLicenseRequired):
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage("must accept the data license agreement"))
	case errors.Is(err, ErrTokenInvalid):
		httpx.Fail(c, httpx.ErrForbidden.WithMessage("download link is invalid, expired, or used up"))
	default:
		httpx.Fail(c, httpx.ErrInternal)
	}
}
