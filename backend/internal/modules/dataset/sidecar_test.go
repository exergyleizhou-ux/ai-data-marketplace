package dataset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/quality"
)

func TestHTTPScreenerMapsResponse(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/screen" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"schema_version":"1.0",
			"engine":{"paperguard_version":"2.17.0","detectors_run":8},
			"summary":{"authenticity_score":72,"band":"review","n_findings":2,"columns_screened":3,"rows":500,"truncated":false},
			"findings":[{"detector":"A2","severity":"SUSPICIOUS"}],
			"errors":[]
		}`))
	}))
	defer srv.Close()

	sc := newHTTPScreener(srv.URL, 5*time.Second)
	chk, err := sc.Screen(context.Background(), []byte("a,b\n1,2\n"), "text/csv")
	if err != nil {
		t.Fatalf("Screen error: %v", err)
	}
	if gotBody != "a,b\n1,2\n" {
		t.Errorf("sidecar did not receive content, got %q", gotBody)
	}
	if chk.Type != quality.TypeAuthenticity {
		t.Errorf("type = %s", chk.Type)
	}
	if chk.Result != quality.ResultWarn { // band=review -> warn
		t.Errorf("result = %s, want warn", chk.Result)
	}
	if chk.Report["band"] != "review" || chk.Report["score"] != 72 {
		t.Errorf("report mismatch: %v", chk.Report)
	}
	if chk.Report["engine"] != "paperguard-sidecar" {
		t.Errorf("engine not tagged: %v", chk.Report["engine"])
	}
}

func TestHTTPScreenerCleanBandPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"summary":{"authenticity_score":95,"band":"clean"}}`))
	}))
	defer srv.Close()
	chk, err := newHTTPScreener(srv.URL, time.Second).Screen(context.Background(), []byte("x\n1\n"), "text/csv")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if chk.Result != quality.ResultPass {
		t.Errorf("clean band must pass, got %s", chk.Result)
	}
}

func TestHTTPScreenerErrorsPropagateForFallback(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); _, _ = w.Write([]byte("boom")) }},
		{"malformed json", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) }},
		{"missing band", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"summary":{}}`)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			if _, err := newHTTPScreener(srv.URL, time.Second).Screen(context.Background(), []byte("x\n"), "text/csv"); err == nil {
				t.Errorf("%s: expected an error so the worker falls back to the Go baseline", c.name)
			}
		})
	}
}

func TestHTTPScreenerUnreachableErrors(t *testing.T) {
	// A dead address must error promptly (and the worker falls back), not hang.
	sc := newHTTPScreener("http://127.0.0.1:0", 500*time.Millisecond)
	if _, err := sc.Screen(context.Background(), []byte("x\n"), "text/csv"); err == nil {
		t.Error("unreachable sidecar must return an error")
	}
}

// Compile-time check that *httpScreener satisfies the interface the Service uses.
var _ authenticityScreener = (*httpScreener)(nil)

func TestScreenContentTypeForwarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Content-Type"), "csv") {
			t.Errorf("content-type not forwarded: %q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"summary":{"band":"clean","authenticity_score":100}}`))
	}))
	defer srv.Close()
	_, _ = newHTTPScreener(srv.URL, time.Second).Screen(context.Background(), []byte("x\n"), "text/csv")
}
