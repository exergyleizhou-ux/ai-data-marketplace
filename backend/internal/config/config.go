// Package config loads runtime configuration from environment variables.
// In later PRs this grows (payment provider keys, OSS creds, JWT secrets);
// keep secrets out of source control — see .env.example.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env         string // development | staging | production
	HTTPAddr    string // e.g. ":8080"
	DatabaseURL string // postgres DSN
	RedisURL    string // redis URL
	AutoMigrate bool   // run DB migrations on startup (handy for dev/CI)

	JWTSecret     string        // HMAC signing secret for JWTs
	JWTAccessTTL  time.Duration // access-token lifetime
	JWTRefreshTTL time.Duration // refresh-token lifetime

	PIISecret      string // keyed-hash secret for sensitive fields (e.g. ID numbers)
	KYCAutoApprove bool   // dev only: auto-verify KYC submissions instead of manual review

	StorageDriver string // local | oss
	StorageDir    string // base dir for the local storage driver

	PaymentProvider   string // mock (sandbox) | wechat | alipay (real = Spike-2 + 法务)
	PaymentMockSecret string // HMAC secret for the sandbox provider's callbacks

	CORSAllowOrigin string // browser origin allowed to call the API ("*" in dev)
}

// Load reads configuration from the environment, applying sane local-dev
// defaults. It returns an error only for malformed values, not missing ones.
func Load() (*Config, error) {
	cfg := &Config{
		Env:         getenv("APP_ENV", "development"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://app:app@localhost:5432/ai_data_marketplace?sslmode=disable"),
		RedisURL:    getenv("REDIS_URL", "redis://localhost:6379/0"),
		AutoMigrate: getenv("AUTO_MIGRATE", "false") == "true",

		// Dev default secret — MUST be overridden in staging/production.
		JWTSecret:     getenv("JWT_SECRET", "dev-insecure-change-me"),
		JWTAccessTTL:  15 * time.Minute,
		JWTRefreshTTL: 30 * 24 * time.Hour,

		PIISecret:      getenv("PII_SECRET", "dev-pii-secret"),
		KYCAutoApprove: getenv("KYC_AUTO_APPROVE", "false") == "true",

		StorageDriver: getenv("STORAGE_DRIVER", "local"),
		StorageDir:    getenv("STORAGE_DIR", "./data/storage"),

		PaymentProvider:   getenv("PAYMENT_PROVIDER", "mock"),
		PaymentMockSecret: getenv("PAYMENT_MOCK_SECRET", "dev-pay-secret"),

		CORSAllowOrigin: getenv("CORS_ALLOW_ORIGIN", "*"),
	}
	// Validate that any provided PORT-style override parses, to fail fast.
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf("invalid HTTP_PORT %q: %w", v, err)
		}
		cfg.HTTPAddr = ":" + v
	}
	// Never run production with the insecure dev secret.
	if cfg.Env == "production" && cfg.JWTSecret == "dev-insecure-change-me" {
		return nil, fmt.Errorf("JWT_SECRET must be set in production")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
