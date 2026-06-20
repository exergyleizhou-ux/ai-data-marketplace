package compute

import (
	"fmt"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/apikey"
	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// RegisterVerifyAPI mounts the self-serve, API-key-metered Verify endpoint
// (Oasis Verify). A paying caller POSTs a dataset and gets back an integrity
// report + a re-hash-verifiable certificate — the marketplace machinery is not
// involved. Mounted at POST /screen (not /verify/screen, which would collide with
// the /verify/:cert_id wildcard).
func RegisterVerifyAPI(rg *gin.RouterGroup, svc *Service, apiKeyMW gin.HandlerFunc) {
	h := &verifyAPI{svc: svc}
	g := rg.Group("/screen")
	g.Use(apiKeyMW)
	g.POST("", h.screen)
}

type verifyAPI struct{ svc *Service }

// screen runs the integrity screen on the uploaded dataset (multipart "file" or
// the raw request body) and returns the report + certificate.
func (h *verifyAPI) screen(c *gin.Context) {
	data, err := readUpload(c)
	if err != nil {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(err.Error()))
		return
	}
	// Enforce the caller's tier size cap.
	if max := apikey.TierOf(apikey.KeyTier(c)).MaxBytes; int64(len(data)) > max {
		httpx.Fail(c, httpx.ErrInvalidParam.WithMessage(fmt.Sprintf("dataset exceeds your plan limit of %d bytes", max)))
		return
	}
	res, err := h.svc.ScreenAdhoc(c.Request.Context(), apikey.AccountID(c), data)
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage(err.Error()))
		return
	}
	httpx.OK(c, res)
}

// readUpload reads the dataset from a multipart "file" field or, failing that,
// the raw request body. Bounded to avoid unbounded buffering.
func readUpload(c *gin.Context) ([]byte, error) {
	const max = 32 << 20
	if f, _, err := c.Request.FormFile("file"); err == nil {
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, max))
	}
	if c.Request.Body == nil {
		return nil, fmt.Errorf("no dataset uploaded (use multipart 'file' or a raw body)")
	}
	b, err := io.ReadAll(io.LimitReader(c.Request.Body, max))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("no dataset uploaded (use multipart 'file' or a raw body)")
	}
	return b, nil
}
