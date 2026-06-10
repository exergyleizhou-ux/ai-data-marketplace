package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name  string
		env   string
		check func(t *testing.T, w *httptest.ResponseRecorder)
	}{
		{
			name: "production sets CSP enforce",
			env:  "production",
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if v := w.Header().Get("Content-Security-Policy"); v == "" {
					t.Error("production must set Content-Security-Policy")
				}
				if v := w.Header().Get("Content-Security-Policy-Report-Only"); v != "" {
					t.Error("production must NOT use report-only CSP")
				}
			},
		},
		{
			name: "development sets CSP report-only",
			env:  "development",
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if v := w.Header().Get("Content-Security-Policy-Report-Only"); v == "" {
					t.Error("development must use report-only CSP")
				}
				if v := w.Header().Get("Content-Security-Policy"); v != "" {
					t.Error("development must NOT set enforcing CSP")
				}
			},
		},
		{
			name: "X-Content-Type-Options",
			env:  "production",
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
					t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
				}
			},
		},
		{
			name: "X-Frame-Options",
			env:  "production",
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
					t.Errorf("X-Frame-Options = %q, want DENY", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(SecurityHeaders(tt.env))
			router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			tt.check(t, w)
		})
	}
}

func TestCacheControlHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CacheControl())
	router.GET("/api/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Errorf("Cache-Control = %q, want no-store, max-age=0", got)
	}
}

func TestRemoveServerHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RemoveServerHeader())
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Server"); got != "" {
		t.Errorf("Server header = %q, want empty", got)
	}
}
