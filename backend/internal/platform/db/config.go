package db

import (
	"os"
	"strconv"
	"time"
)

// envInt32 reads a positive int32 from the environment, falling back to def
// on unset, unparsable, or non-positive values — a typo'd deploy manifest
// must degrade to defaults, never to a zero-sized pool.
func envInt32(name string, def int32) int32 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n <= 0 {
		return def
	}
	return int32(n)
}

// envDuration reads a positive time.Duration ("30m", "1h") with the same
// fail-to-default semantics as envInt32.
func envDuration(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
