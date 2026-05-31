// Package redis holds the shared go-redis client constructor. Redis is used for
// rate limiting (PR-06) and later for distributed locks and the refresh-token
// denylist. A failure to connect is non-fatal — callers fall back (e.g. to an
// in-memory rate limiter).
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// New parses the URL, connects, and verifies with a ping. Returns an error if
// Redis is unreachable so the caller can decide whether to degrade.
func New(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
