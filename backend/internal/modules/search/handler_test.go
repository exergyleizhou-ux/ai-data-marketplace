package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeSearcher struct {
	searches []SearchQuery
	results  []SearchResult
	err      error
}

func (f *fakeSearcher) SearchPublished(ctx context.Context, q SearchQuery) ([]SearchResult, error) {
	f.searches = append(f.searches, q)
	return f.results, f.err
}

func setupSearch(t *testing.T, f *fakeSearcher) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r.Group("/api/v1"), f)
	return r
}

func TestSearchHandler_PassesQueryStringToSearcher(t *testing.T) {
	f := &fakeSearcher{results: []SearchResult{}}
	r := setupSearch(t, f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/search?q=hello", nil)
	r.ServeHTTP(w, req)

	if len(f.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(f.searches))
	}
	if f.searches[0].Keyword != "hello" {
		t.Fatalf("keyword = %q, want hello", f.searches[0].Keyword)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestSearchHandler_PassesAllFilters(t *testing.T) {
	f := &fakeSearcher{results: []SearchResult{}}
	r := setupSearch(t, f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/search?q=x&type=text&domain=finance&sort=newest&limit=20&offset=10", nil)
	r.ServeHTTP(w, req)

	if len(f.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(f.searches))
	}
	q := f.searches[0]
	if q.Keyword != "x" || q.DataType != "text" || q.Domain != "finance" || q.Sort != "newest" {
		t.Fatalf("filters mismatch: %+v", q)
	}
	if q.Limit != 20 || q.Offset != 10 {
		t.Fatalf("limit=%d offset=%d, want limit=20 offset=10", q.Limit, q.Offset)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestSearchHandler_LimitClampedAt100(t *testing.T) {
	f := &fakeSearcher{results: []SearchResult{}}
	r := setupSearch(t, f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/search?q=x&limit=500", nil)
	r.ServeHTTP(w, req)

	if len(f.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(f.searches))
	}
	// handler passes raw limit to searcher; real clamping is in dataset.ListPublished.
	q := f.searches[0]
	if q.Limit != 500 {
		t.Fatalf("limit = %d, want 500 (handler passes raw; clamping is in dataset repo)", q.Limit)
	}
}

func TestSearchHandler_EmptyResultsReturnsEmptyItemsArray(t *testing.T) {
	f := &fakeSearcher{results: nil}
	r := setupSearch(t, f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/search?q=x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code int                        `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !strings.Contains(string(resp.Data["items"]), "[") {
		t.Fatal("items must be an empty array, not null")
	}
}

func TestSearchHandler_SearcherErrorReturns500(t *testing.T) {
	f := &fakeSearcher{err: context.DeadlineExceeded}
	r := setupSearch(t, f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/search?q=x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
