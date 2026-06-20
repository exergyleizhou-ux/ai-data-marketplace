// Package config loads runtime configuration from environment variables.
// In later PRs this grows (payment provider keys, OSS creds, JWT secrets);
// keep secrets out of source control — see .env.example.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
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

	StorageDriver string // local | s3  (s3 = MinIO / AWS S3 / Aliyun OSS / Tencent COS)
	StorageDir    string // base dir for the local storage driver
	S3Endpoint    string // host:port (no scheme)
	S3Bucket      string
	S3AccessKey   string
	S3SecretKey   string
	S3UseSSL      bool
	S3Region      string

	PaymentProvider     string // mock (sandbox) | stripe | wechat | alipay (wechat/alipay real = Spike-2 + 法务)
	PaymentMockSecret   string // HMAC secret for the sandbox provider's callbacks
	StripeSecretKey     string // sk_test_… (test mode is free, no real money)
	StripeWebhookSecret string // whsec_… from `stripe listen` / dashboard endpoint
	StripeCurrency      string // settlement currency for Stripe (test default usd)
	VerifyPriceMap      string // Oasis Verify: "priceID:tier,priceID:tier" (Stripe price → plan tier)

	CORSAllowOrigin string // browser origin allowed to call the API ("*" in dev)
	AppBaseURL      string // frontend base URL for email links (e.g. https://app.example.com)
	MetricsToken    string // optional bearer token to protect /metrics (empty = open, dev only)

	// TrustedProxies are the networks whose X-Forwarded-For header is honored
	// when resolving the client IP (used as the per-IP rate-limit key). A
	// request arriving from outside these ranges cannot forge its source IP.
	TrustedProxies []string
}

// DefaultTrustedProxies are the networks a reverse proxy (Caddy/nginx/LB)
// typically occupies: loopback, RFC1918 private ranges, and the IPv6
// loopback/unique-local ranges. Trusting only these means gin resolves the
// real client from X-Forwarded-For ONLY when the request actually transited a
// proxy on one of these networks, and ignores a client-forged XFF arriving
// from the public internet. Override with TRUSTED_PROXIES for other topologies.
func DefaultTrustedProxies() []string {
	return []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"::1/128", "fc00::/7",
	}
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

		// No default JWT secret — in dev a random secret is generated on every
		// start (tokens invalidate across restarts). For persistent tokens in
		// dev, set JWT_SECRET explicitly. Production MUST set JWT_SECRET.
		JWTSecret:     getenv("JWT_SECRET", ""),
		JWTAccessTTL:  15 * time.Minute,
		JWTRefreshTTL: 30 * 24 * time.Hour,

		PIISecret:      getenv("PII_SECRET", "dev-pii-secret"),
		KYCAutoApprove: getenv("KYC_AUTO_APPROVE", "false") == "true",

		StorageDriver: getenv("STORAGE_DRIVER", "local"),
		StorageDir:    getenv("STORAGE_DIR", "./data/storage"),
		S3Endpoint:    getenv("S3_ENDPOINT", "localhost:9000"),
		S3Bucket:      getenv("S3_BUCKET", "ai-data-marketplace"),
		S3AccessKey:   getenv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:   getenv("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:      getenv("S3_USE_SSL", "false") == "true",
		S3Region:      getenv("S3_REGION", "us-east-1"),

		PaymentProvider:     getenv("PAYMENT_PROVIDER", "mock"),
		PaymentMockSecret:   getenv("PAYMENT_MOCK_SECRET", "dev-pay-secret"),
		StripeSecretKey:     getenv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getenv("STRIPE_WEBHOOK_SECRET", ""),
		StripeCurrency:      getenv("STRIPE_CURRENCY", "usd"),
		VerifyPriceMap:      getenv("VERIFY_PRICE_MAP", ""),

		CORSAllowOrigin: getenv("CORS_ALLOW_ORIGIN", "*"),
		AppBaseURL:      getenv("APP_BASE_URL", "http://localhost:3000"),
		MetricsToken:    getenv("METRICS_TOKEN", ""),
		TrustedProxies:  trustedProxiesFromEnv(),
	}
	// Validate that any provided PORT-style override parses, to fail fast.
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf("invalid HTTP_PORT %q: %w", v, err)
		}
		cfg.HTTPAddr = ":" + v
	}
	// JWT_SECRET is mandatory in all environments.
	if cfg.JWTSecret == "" {
		if cfg.Env == "production" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		// Dev/CI: auto-generate a per-startup random secret so the app is
		// usable out of the box but not trivially forgeable via a public default.
		cfg.JWTSecret = randomHex(64)
		fmt.Fprintln(os.Stderr, "WARNING: JWT_SECRET not set — generated random secret for this session. Tokens will NOT survive a restart.")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// trustedProxiesFromEnv returns the TRUSTED_PROXIES override (comma-separated
// IPs/CIDRs) or the safe default when unset.
func trustedProxiesFromEnv() []string {
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		return splitCSV(v)
	}
	return DefaultTrustedProxies()
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// randomHex returns n random bytes hex-encoded, or panics (crypto/rand is
// infallible on modern kernels). For startup-only use — not a hot path.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
