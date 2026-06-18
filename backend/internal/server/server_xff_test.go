package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/config"
)

// clientIPProbe registers a route that echoes gin's resolved client IP, so the
// tests below can observe exactly what value the per-IP rate limiter would key
// on (middleware.KeyByIP returns c.ClientIP()).
func clientIPProbe(s *Server) {
	s.engine.GET("/__clientip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
}

// A request that arrives directly from a public peer (no trusted proxy in
// front) must NOT let the caller forge their source IP via X-Forwarded-For.
// Otherwise the per-IP rate limiter is trivially bypassed by incrementing the
// header (credential stuffing / account enumeration on login, password-reset,
// 2FA). gin's default trusts ALL proxies and echoes the client-supplied XFF,
// so this fails until the server pins trusted proxies.
func TestClientIP_IgnoresSpoofedXFFFromUntrustedPeer(t *testing.T) {
	s := newTestServer()
	clientIPProbe(s)

	req := httptest.NewRequest(http.MethodGet, "/__clientip", nil)
	req.RemoteAddr = "203.0.113.9:44321" // public peer, not a trusted proxy
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want real peer 203.0.113.9 — a spoofed X-Forwarded-For from an untrusted peer must be ignored", got)
	}
}

// The fix must NOT break the real deployment, where a reverse proxy (on a
// trusted private/loopback network) forwards the genuine client IP via
// X-Forwarded-For. There, ClientIP() must resolve the forwarded client so the
// limiter keys per real user, not per proxy.
func TestClientIP_HonorsXFFFromTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New(&config.Config{Env: "test", TrustedProxies: config.DefaultTrustedProxies()}, nil)
	clientIPProbe(s)

	req := httptest.NewRequest(http.MethodGet, "/__clientip", nil)
	req.RemoteAddr = "127.0.0.1:8080" // trusted proxy (loopback)
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want forwarded client 198.51.100.7 — XFF from a trusted proxy must be honored", got)
	}
}
