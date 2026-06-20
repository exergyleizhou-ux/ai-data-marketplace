package apikey

import (
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	plain, prefix, hash, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(plain, "sk_live_") {
		t.Errorf("plaintext should start with sk_live_, got %q", plain[:8])
	}
	if prefix != plain[:keyPrefixLen] {
		t.Errorf("prefix %q should be the first %d chars of %q", prefix, keyPrefixLen, plain)
	}
	if hash != HashKey(plain) {
		t.Errorf("hash should equal HashKey(plaintext)")
	}
	if len(hash) != 64 {
		t.Errorf("hash should be 64 hex chars, got %d", len(hash))
	}
	// The plaintext must carry enough entropy that two keys never collide.
	plain2, _, _, _ := GenerateKey()
	if plain == plain2 {
		t.Errorf("two generated keys must differ")
	}
}

func TestHashKey_DeterministicAndOpaque(t *testing.T) {
	const k = "sk_live_abc123"
	if HashKey(k) != HashKey(k) {
		t.Error("HashKey must be deterministic")
	}
	if strings.Contains(HashKey(k), k) {
		t.Error("hash must not contain the plaintext")
	}
}

func TestTiers(t *testing.T) {
	free, ok := Tiers["free"]
	if !ok {
		t.Fatal("free tier must exist")
	}
	if free.MonthlyQuota <= 0 || free.MaxBytes <= 0 {
		t.Errorf("free tier needs positive quota+size, got %+v", free)
	}
	if Tiers["pro"].MonthlyQuota <= free.MonthlyQuota {
		t.Error("pro quota should exceed free")
	}
}
