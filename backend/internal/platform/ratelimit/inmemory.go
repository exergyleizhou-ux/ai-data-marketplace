package ratelimit

import (
	"context"
	"sync"
	"time"
)

type counter struct {
	count   int
	resetAt time.Time
}

// InMemory is a process-local fixed-window limiter. Suitable for a single
// instance or as a fallback; for multi-instance deployments use Redis so the
// window is shared.
type InMemory struct {
	mu   sync.Mutex
	keys map[string]*counter
	now  func() time.Time // injectable for tests
}

// NewInMemory creates the limiter and starts a background janitor that evicts
// expired keys so the map does not grow unbounded.
func NewInMemory() *InMemory {
	m := &InMemory{keys: make(map[string]*counter), now: time.Now}
	go m.janitor()
	return m
}

func (m *InMemory) Allow(_ context.Context, key string, limit int, window time.Duration) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	c, ok := m.keys[key]
	if !ok || now.After(c.resetAt) {
		c = &counter{count: 0, resetAt: now.Add(window)}
		m.keys[key] = c
	}
	c.count++

	remaining := limit - c.count
	if remaining < 0 {
		remaining = 0
	}
	if c.count > limit {
		return Result{Allowed: false, Remaining: 0, RetryAfter: c.resetAt.Sub(now)}, nil
	}
	return Result{Allowed: true, Remaining: remaining}, nil
}

func (m *InMemory) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := m.now()
		m.mu.Lock()
		for k, c := range m.keys {
			if now.After(c.resetAt) {
				delete(m.keys, k)
			}
		}
		m.mu.Unlock()
	}
}
