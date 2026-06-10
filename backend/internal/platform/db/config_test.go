package db

import (
	"testing"
	"time"
)

func TestEnvInt32_DefaultAndOverride(t *testing.T) {
	t.Setenv("DB_TEST_INT", "")
	if got := envInt32("DB_TEST_INT", 10); got != 10 {
		t.Fatalf("default: got %d, want 10", got)
	}
	t.Setenv("DB_TEST_INT", "25")
	if got := envInt32("DB_TEST_INT", 10); got != 25 {
		t.Fatalf("override: got %d, want 25", got)
	}
	// Garbage and non-positive values fall back to the default — a typo in a
	// deploy manifest must not zero out the pool.
	for _, bad := range []string{"abc", "0", "-3"} {
		t.Setenv("DB_TEST_INT", bad)
		if got := envInt32("DB_TEST_INT", 10); got != 10 {
			t.Fatalf("bad value %q: got %d, want default 10", bad, got)
		}
	}
}

func TestEnvDuration_DefaultAndOverride(t *testing.T) {
	t.Setenv("DB_TEST_DUR", "")
	if got := envDuration("DB_TEST_DUR", time.Hour); got != time.Hour {
		t.Fatalf("default: got %v, want 1h", got)
	}
	t.Setenv("DB_TEST_DUR", "30m")
	if got := envDuration("DB_TEST_DUR", time.Hour); got != 30*time.Minute {
		t.Fatalf("override: got %v, want 30m", got)
	}
	for _, bad := range []string{"nope", "-5m", "0"} {
		t.Setenv("DB_TEST_DUR", bad)
		if got := envDuration("DB_TEST_DUR", time.Hour); got != time.Hour {
			t.Fatalf("bad value %q: got %v, want default 1h", bad, got)
		}
	}
}
