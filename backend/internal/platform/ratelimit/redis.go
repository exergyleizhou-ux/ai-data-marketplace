package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// fixedWindowScript atomically increments the key's counter and sets the
// window TTL only on the first hit, then returns {count, pttl_ms}. Doing this
// in one Lua call avoids a race between INCR and EXPIRE.
var fixedWindowScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {c, ttl}
`)

// Redis is a fixed-window limiter backed by a shared Redis instance, so the
// window is consistent across all app instances.
type Redis struct{ c *redis.Client }

func NewRedis(c *redis.Client) *Redis { return &Redis{c: c} }

func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	res, err := fixedWindowScript.Run(ctx, r.c, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return Result{}, err
	}
	count, ttlMS := res[0], res[1]

	retryAfter := time.Duration(ttlMS) * time.Millisecond
	if ttlMS < 0 { // -1 (no expiry) / -2 (missing) — fall back to the full window
		retryAfter = window
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	if count > int64(limit) {
		return Result{Allowed: false, Remaining: 0, RetryAfter: retryAfter}, nil
	}
	return Result{Allowed: true, Remaining: remaining}, nil
}
