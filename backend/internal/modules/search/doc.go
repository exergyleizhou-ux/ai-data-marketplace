// Package search provides browse/filter/keyword search over published datasets.
//
// Boundary: read-only over published dataset projections. P3 uses PostgreSQL
// full-text search with a Chinese tokenizer (zhparser / pg_jieba) — NOT
// pg_trgm, which is ineffective for word-boundary-free Chinese. Migrate to
// Elasticsearch + IK analyzer once volume warrants (P2).
//
// Implemented in: PR-16 (Chinese FTS + filters).
package search
