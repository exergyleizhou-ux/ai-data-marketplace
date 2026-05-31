package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryFixedWindow(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Build directly (no janitor goroutine) with an injectable clock.
	m := &InMemory{keys: map[string]*counter{}, now: func() time.Time { return clock }}
	ctx := context.Background()
	const limit = 3
	const window = time.Minute

	for i := 1; i <= limit; i++ {
		res, err := m.Allow(ctx, "k", limit, window)
		if err != nil {
			t.Fatalf("allow %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i)
		}
		if want := limit - i; res.Remaining != want {
			t.Errorf("request %d remaining = %d, want %d", i, res.Remaining, want)
		}
	}

	// One past the limit is denied with a positive RetryAfter.
	res, _ := m.Allow(ctx, "k", limit, window)
	if res.Allowed {
		t.Fatal("request over limit should be denied")
	}
	if res.RetryAfter <= 0 || res.RetryAfter > window {
		t.Errorf("RetryAfter = %v, want (0, %v]", res.RetryAfter, window)
	}

	// A different key is independent.
	if res, _ := m.Allow(ctx, "other", limit, window); !res.Allowed {
		t.Error("independent key should be allowed")
	}

	// After the window elapses, the key resets.
	clock = clock.Add(window + time.Second)
	if res, _ := m.Allow(ctx, "k", limit, window); !res.Allowed {
		t.Error("request after window reset should be allowed")
	}
}
