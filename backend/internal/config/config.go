// Package config loads runtime configuration from environment variables.
// In later PRs this grows (payment provider keys, OSS creds, JWT secrets);
// keep secrets out of source control — see .env.example.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Env         string // development | staging | production
	HTTPAddr    string // e.g. ":8080"
	DatabaseURL string // postgres DSN
	RedisURL    string // redis URL
}

// Load reads configuration from the environment, applying sane local-dev
// defaults. It returns an error only for malformed values, not missing ones.
func Load() (*Config, error) {
	cfg := &Config{
		Env:         getenv("APP_ENV", "development"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://app:app@localhost:5432/ai_data_marketplace?sslmode=disable"),
		RedisURL:    getenv("REDIS_URL", "redis://localhost:6379/0"),
	}
	// Validate that any provided PORT-style override parses, to fail fast.
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf("invalid HTTP_PORT %q: %w", v, err)
		}
		cfg.HTTPAddr = ":" + v
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
