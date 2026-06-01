package auth

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Denylist tracks revoked refresh-token IDs (jti) so a logged-out or rotated
// token can never be replayed, even though it is still a structurally valid,
// unexpired JWT. Entries are kept only until the token's own expiry (the TTL),
// which bounds storage.
//
// Two backends mirror the rate limiter: Redis (shared across instances) and an
// in-memory fallback (single instance) for when Redis is unavailable. Access
// tokens are deliberately NOT checked against the denylist — they are short
// lived (minutes) and checking every request would add a Redis hop to the hot
// path; revoking the refresh token stops the session from being renewed.
type Denylist interface {
	// Revoke marks jti as revoked for ttl. A ttl <= 0 is a no-op (the token has
	// already expired and cannot be used regardless).
	Revoke(ctx context.Context, jti string, ttl time.Duration) error
	// IsRevoked reports whether jti is currently revoked.
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

// noopDenylist is the default when no backend is configured: nothing is ever
// revoked. It keeps Refresh working (and tests simple) without a Redis or
// background goroutine, preserving the pre-H4 stateless behaviour.
type noopDenylist struct{}

func (noopDenylist) Revoke(context.Context, string, time.Duration) error { return nil }
func (noopDenylist) IsRevoked(context.Context, string) (bool, error)     { return false, nil }

const denylistKeyPrefix = "auth:revoked_jti:"

// RedisDenylist stores revoked jtis as keys with a TTL, shared across instances.
type RedisDenylist struct{ c *redis.Client }

// NewRedisDenylist wraps a go-redis client.
func NewRedisDenylist(c *redis.Client) *RedisDenylist { return &RedisDenylist{c: c} }

func (d *RedisDenylist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" || ttl <= 0 {
		return nil
	}
	return d.c.Set(ctx, denylistKeyPrefix+jti, "1", ttl).Err()
}

func (d *RedisDenylist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	n, err := d.c.Exists(ctx, denylistKeyPrefix+jti).Result()
	return n > 0, err
}

// InMemoryDenylist is a process-local denylist with a janitor that evicts
// expired entries. Suitable for a single instance or as a Redis fallback; note
// revocations are lost on restart and not shared across instances.
type InMemoryDenylist struct {
	mu      sync.Mutex
	expires map[string]time.Time
	now     func() time.Time // injectable for tests
}

// NewInMemoryDenylist creates the denylist and starts its eviction janitor.
func NewInMemoryDenylist() *InMemoryDenylist {
	d := &InMemoryDenylist{expires: make(map[string]time.Time), now: time.Now}
	go d.janitor()
	return d
}

func (d *InMemoryDenylist) Revoke(_ context.Context, jti string, ttl time.Duration) error {
	if jti == "" || ttl <= 0 {
		return nil
	}
	d.mu.Lock()
	d.expires[jti] = d.now().Add(ttl)
	d.mu.Unlock()
	return nil
}

func (d *InMemoryDenylist) IsRevoked(_ context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	exp, ok := d.expires[jti]
	if !ok {
		return false, nil
	}
	if d.now().After(exp) {
		delete(d.expires, jti)
		return false, nil
	}
	return true, nil
}

func (d *InMemoryDenylist) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := d.now()
		d.mu.Lock()
		for jti, exp := range d.expires {
			if now.After(exp) {
				delete(d.expires, jti)
			}
		}
		d.mu.Unlock()
	}
}
