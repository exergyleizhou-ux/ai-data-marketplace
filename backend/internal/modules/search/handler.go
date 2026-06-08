package search

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// DatasetSearcher is the search index the search module reads from.
// Implemented by the dataset module and injected by the server so
// neither package imports the other.
type DatasetSearcher interface {
	SearchPublished(ctx context.Context, q SearchQuery) ([]SearchResult, error)
}

// SearchQuery carries the user's search parameters.
type SearchQuery struct {
	Keyword  string
	DataType string
	Domain   string
	Sort     string
	Limit    int
	Offset   int
}

// SearchResult is a lightweight published-dataset hit.
type SearchResult struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	DataType         string  `json:"data_type"`
	Domain           string  `json:"domain"`
	PriceCents       int64   `json:"price_cents"`
	Status           string  `json:"status"`
	AuthenticityBand string  `json:"authenticity_band,omitempty"`
	Relevance        float64 `json:"relevance,omitempty"`
}

type handler struct{ searcher DatasetSearcher }

// Register mounts the search endpoint.
func Register(rg *gin.RouterGroup, searcher DatasetSearcher) {
	h := &handler{searcher: searcher}
	rg.GET("/search", h.search)
}

func (h *handler) search(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.searcher.SearchPublished(c.Request.Context(), SearchQuery{
		Keyword:  c.Query("q"),
		DataType: c.Query("type"),
		Domain:   c.Query("domain"),
		Sort:     c.Query("sort"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal.WithMessage(err.Error()))
		return
	}
	if items == nil {
		items = []SearchResult{}
	}
	httpx.OK(c, gin.H{"items": items})
}
