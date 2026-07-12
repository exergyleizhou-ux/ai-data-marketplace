package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFRejectsCrossSite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", CSRF(), func(c *gin.Context) { c.Status(204) })
	q := httptest.NewRequest("POST", "http://oasis.test/", nil)
	q.Host = "oasis.test"
	q.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}
func TestCSRFRequiresDoubleSubmitForSession(t *testing.T) {
	r := gin.New()
	r.POST("/", CSRF(), func(c *gin.Context) { c.Status(204) })
	for _, ok := range []bool{false, true} {
		q := httptest.NewRequest("POST", "http://oasis.test/", nil)
		q.Host = "oasis.test"
		q.Header.Set("Origin", "http://oasis.test")
		q.AddCookie(&http.Cookie{Name: "oasis_refresh", Value: "r"})
		q.AddCookie(&http.Cookie{Name: "oasis_csrf", Value: "c"})
		if ok {
			q.Header.Set("X-CSRF-Token", "c")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		want := http.StatusForbidden
		if ok {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("ok=%v status=%d", ok, w.Code)
		}
	}
}
