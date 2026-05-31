// Package textseg provides Chinese word segmentation for full-text search.
//
// Chinese has no spaces between words, so PostgreSQL's default tokenizers (and
// pg_trgm) can't index it meaningfully (docs §0/§6.4). The doc's planned
// `zhparser` is a C extension that can't be installed everywhere; instead we
// segment in the application with a pure-Go jieba-style tokenizer (gse) and
// feed space-joined tokens into a PostgreSQL `simple` tsvector + GIN index.
// This gives real word-level, ranked search. zhparser / Elasticsearch+IK remain
// drop-in alternatives at scale.
package textseg

import (
	"strings"
	"sync"

	"github.com/go-ego/gse"
)

var (
	seg   gse.Segmenter
	once  sync.Once
	ready bool
)

func ensure() {
	once.Do(func() {
		if err := seg.LoadDictEmbed("zh_s"); err == nil {
			ready = true
		}
	})
}

// Segment returns the input as space-joined word tokens, suitable for building
// a `to_tsvector('simple', ...)` or `plainto_tsquery('simple', ...)`. If the
// dictionary failed to load it degrades to the raw input (still works for
// latin/space-separated text).
func Segment(s string) string {
	ensure()
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !ready {
		return s
	}
	// Search mode (jieba cut-for-search): emits overlapping/sub tokens (e.g.
	// 语料库 -> 语料, 语料库) for better recall when indexing and querying.
	return strings.Join(seg.CutSearch(s, true), " ")
}
