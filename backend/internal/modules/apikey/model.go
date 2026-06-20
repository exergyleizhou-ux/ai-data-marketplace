// Package apikey provides self-serve API keys for the Oasis Verify product —
// the metered, billable surface that lets a developer authenticate to the public
// verification API without a full marketplace session. Keys are stored only as a
// SHA-256 hash; the plaintext is shown exactly once at issue time.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// keyPrefixLen is how many leading chars of the plaintext we keep for display /
// narrowing ("sk_live_" + 8 chars).
const keyPrefixLen = 16

// Tier is a billing plan: a monthly scan quota + a per-dataset size cap.
type Tier struct {
	Name         string
	MonthlyQuota int   // scans per calendar month
	MaxBytes     int64 // max dataset size per scan
}

// Tiers are the MVP plans (free self-serve → paid). Stripe maps price IDs to
// these names; the quota is enforced in the service.
var Tiers = map[string]Tier{
	"free":  {Name: "free", MonthlyQuota: 5, MaxBytes: 5 << 20},
	"pro":   {Name: "pro", MonthlyQuota: 500, MaxBytes: 100 << 20},
	"scale": {Name: "scale", MonthlyQuota: 100_000, MaxBytes: 2 << 30},
}

// TierOf returns the tier config for a name, defaulting to free.
func TierOf(name string) Tier {
	if t, ok := Tiers[name]; ok {
		return t
	}
	return Tiers["free"]
}

// APIKey is a stored key record (never holds the plaintext).
type APIKey struct {
	ID         string `json:"id"`
	AccountID  string `json:"account_id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"` // "sk_live_xxxxxxxx" — safe to display
	Tier       string `json:"tier"`
	UsageMonth string `json:"usage_month"` // "YYYY-MM" the counter belongs to
	UsageCount int    `json:"usage_count"`
	CreatedAt  string `json:"created_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	RevokedAt  string `json:"revoked_at,omitempty"`
}

// Revoked reports whether the key has been revoked.
func (k APIKey) Revoked() bool { return k.RevokedAt != "" }

// HashKey is the at-rest representation of a key (SHA-256 hex). Lookups compare
// hashes, so the plaintext never needs to be stored or logged.
func HashKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// GenerateKey mints a new key: the one-time plaintext ("sk_live_…"), its display
// prefix, and the at-rest hash. The caller returns the plaintext to the user once
// and persists only the prefix + hash.
func GenerateKey() (plaintext, prefix, hash string, err error) {
	b := make([]byte, 24) // 192 bits of entropy
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	plaintext = "sk_live_" + base64.RawURLEncoding.EncodeToString(b)
	prefix = plaintext[:keyPrefixLen]
	hash = HashKey(plaintext)
	return plaintext, prefix, hash, nil
}
