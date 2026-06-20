package verify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBadgeSVG_RendersLabelMessageColor(t *testing.T) {
	svg := badgeSVG("Oasis C2D", "verified", "#3f7a5c")
	if !strings.HasPrefix(svg, "<svg") {
		t.Fatalf("not an svg: %.40q", svg)
	}
	for _, want := range []string{"Oasis C2D", "verified", "#3f7a5c"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("svg missing %q", want)
		}
	}
}

// A verified cert returns a 200 SVG badge with the verified message — and does
// NOT reflect the (caller-controlled) cert_id into the SVG body (XSS-safe: an
// SVG opened directly in a browser executes its scripts).
func TestBadgeEndpoint_Verified(t *testing.T) {
	repo := fakeRepo{ci: &CertInfo{CertID: "VO-ABCDEF012345", ResourceType: "compute_result", CreatedAt: "2026-06-18T00:00:00Z"}}
	w := httptest.NewRecorder()
	newEngine(repo).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/verify/VO-ABCDEF012345/badge.svg", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("content-type = %q, want image/svg+xml", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") || !strings.Contains(body, "verified") {
		t.Fatalf("badge body unexpected: %.80q", body)
	}
	if strings.Contains(body, "VO-ABCDEF012345") {
		t.Fatalf("badge must not reflect the caller-controlled cert_id: %s", body)
	}
}

// A missing cert still returns a 200 SVG (graceful degrade for an embedded <img>)
// showing an unverified badge — not a 404.
func TestBadgeEndpoint_NotFoundGraceful(t *testing.T) {
	repo := fakeRepo{err: ErrNotFound}
	w := httptest.NewRecorder()
	newEngine(repo).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/verify/VO-MISSING000000/badge.svg", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unverified") {
		t.Fatalf("missing-cert badge should say unverified: %s", w.Body.String())
	}
}
