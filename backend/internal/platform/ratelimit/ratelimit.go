// Package ratelimit provides a small fixed-window rate limiter behind a single
// interface, with two backends: Redis (shared across instances) and in-memory
// (single-instance fallback when Redis is unavailable). The HTTP middleware
// lives in platform/middleware and depends on this interface.
package ratelimit

import (
	"context"
	"time"
)

// Result describes the outcome of an Allow check.
type Result struct {
	Allowed    bool
	Remaining  int           // requests left in the current window (>= 0)
	RetryAfter time.Duration // when denied, how long until the window resets
}

// Limiter is a fixed-window counter: at most `limit` calls per `window` per key.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}
