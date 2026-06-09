package order

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeBundleSvc struct {
	preflightErr error
}

func (f *fakeBundleSvc) BundlePreflight(_ context.Context, _ string, _ []string) ([]BundleEntry, error) {
	return nil, f.preflightErr
}
func (f *fakeBundleSvc) BundleStream(_ context.Context, _ io.Writer, _ []BundleEntry) error {
	return nil
}

func setupHandler(fsvc *fakeBundleSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/bundle", func(c *gin.Context) {
		var req struct {
			OrderIDs []string `json:"order_ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.OrderIDs) == 0 {
			c.JSON(400, gin.H{"code": 400, "message": "order_ids is required"})
			return
		}
		entries, err := fsvc.BundlePreflight(c.Request.Context(), "test-user", req.OrderIDs)
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Status(http.StatusOK)
		_ = fsvc.BundleStream(c.Request.Context(), c.Writer, entries)
	})
	return r
}

func doBundle(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/bundle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBundleHandler_ForeignOrder_Returns403(t *testing.T) {
	r := setupHandler(&fakeBundleSvc{preflightErr: ErrForbidden})
	w := doBundle(t, r, `{"order_ids":["o1"]}`)
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestBundleHandler_NonSettled_Returns409(t *testing.T) {
	r := setupHandler(&fakeBundleSvc{preflightErr: ErrBadTransition})
	w := doBundle(t, r, `{"order_ids":["o1"]}`)
	if w.Code != 409 {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("Content-Type must be application/json")
	}
}

func TestBundleHandler_ComputeOrder_Returns400(t *testing.T) {
	r := setupHandler(&fakeBundleSvc{preflightErr: ErrValidation})
	w := doBundle(t, r, `{"order_ids":["o1"]}`)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("Content-Type must be application/json")
	}
}

func TestBundleHandler_Success_SetsZipHeaders(t *testing.T) {
	r := setupHandler(&fakeBundleSvc{})
	w := doBundle(t, r, `{"order_ids":["o1"]}`)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != nil {
		t.Fatal("successful bundle must not return a JSON error object")
	}
}
