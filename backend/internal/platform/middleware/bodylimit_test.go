package middleware

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func bodyLimitRouter(defaultMax int64, rules ...BodyLimitRule) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(defaultMax, rules...))
	echo := func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(413, gin.H{"read": "blocked"})
			return
		}
		c.JSON(200, gin.H{"n": len(b)})
	}
	r.POST("/small", echo)
	r.PUT("/datasets/:id/upload/part", echo)
	return r
}

func TestBodyLimit_AllowsUnderLimit(t *testing.T) {
	r := bodyLimitRouter(1024)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/small", strings.NewReader("hello")))
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestBodyLimit_RejectsByContentLength(t *testing.T) {
	r := bodyLimitRouter(1024)
	req := httptest.NewRequest("POST", "/small", bytes.NewReader(make([]byte, 2048)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 413 {
		t.Fatalf("status = %d, want 413", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":1005`) {
		t.Errorf("413 must use the standard envelope with code 1005, got %s", w.Body.String())
	}
}

func TestBodyLimit_BackstopsChunkedBodies(t *testing.T) {
	// No Content-Length (chunked): the fast-reject can't fire, but
	// MaxBytesReader must stop the handler from reading past the limit.
	r := bodyLimitRouter(1024)
	req := httptest.NewRequest("POST", "/small", io.NopCloser(bytes.NewReader(make([]byte, 4096))))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatalf("oversized chunked body must not be fully readable (got 200)")
	}
}

func TestBodyLimit_PerRouteOverride(t *testing.T) {
	r := bodyLimitRouter(1024, BodyLimitRule{Route: "/datasets/:id/upload/part", Max: 1 << 20})
	// 100 KiB: over the 1 KiB default, under the 1 MiB override.
	big := bytes.NewReader(make([]byte, 100<<10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/datasets/abc/upload/part", big))
	if w.Code != 200 {
		t.Fatalf("upload route should accept 100KiB under its override, got %d", w.Code)
	}
	// The same body on a normal route must be rejected.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("POST", "/small", bytes.NewReader(make([]byte, 100<<10))))
	if w2.Code != 413 {
		t.Fatalf("default route must reject 100KiB, got %d", w2.Code)
	}
}
