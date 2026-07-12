package auth

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserCookiesAreHttpOnlyAndScoped(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "https://oasis.test/", nil)
	setBrowserCookies(c, Tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 300})
	joined := strings.Join(w.Header().Values("Set-Cookie"), "\n")
	for _, part := range []string{"oasis_access=a", "oasis_refresh=r", "HttpOnly", "SameSite=Lax", "Path=/api/v1/auth/session", "Secure"} {
		if !strings.Contains(joined, part) {
			t.Fatalf("missing %q in %s", part, joined)
		}
	}
	csrf := joined[strings.Index(joined, "oasis_csrf="):]
	csrf = strings.SplitN(csrf, "\n", 2)[0]
	if strings.Contains(csrf, "HttpOnly") {
		t.Fatal("csrf cookie must be JS-readable for double submit")
	}
}
