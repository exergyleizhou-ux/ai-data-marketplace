package auth

import (
	"strings"
	"testing"
)

const testPIISecret = "unit-test-pii-secret"

func TestEncryptDecryptIDNo_RoundTrip(t *testing.T) {
	const idNo, userID = "110101199001011234", "user-abc"
	blob, err := encryptIDNo(testPIISecret, idNo, userID)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if blob[0] != piiKeyVersion {
		t.Fatalf("version byte = %d, want %d", blob[0], piiKeyVersion)
	}
	if strings.Contains(string(blob), idNo) {
		t.Fatal("plaintext ID number leaked into ciphertext")
	}
	got, err := decryptIDNo(testPIISecret, blob, userID)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != idNo {
		t.Fatalf("round-trip = %q, want %q", got, idNo)
	}
}

func TestDecryptIDNo_WrongUserAADFails(t *testing.T) {
	blob, _ := encryptIDNo(testPIISecret, "110101199001011234", "user-abc")
	// Same key, different AAD (record swapped to another user) must fail.
	if _, err := decryptIDNo(testPIISecret, blob, "user-xyz"); err == nil {
		t.Fatal("decrypt with wrong user AAD should fail authentication")
	}
}

func TestDecryptIDNo_TamperFails(t *testing.T) {
	blob, _ := encryptIDNo(testPIISecret, "110101199001011234", "user-abc")
	blob[len(blob)-1] ^= 0xFF // flip a ciphertext byte
	if _, err := decryptIDNo(testPIISecret, blob, "user-abc"); err == nil {
		t.Fatal("decrypt of tampered ciphertext should fail")
	}
}

func TestDecryptIDNo_WrongKeyFails(t *testing.T) {
	blob, _ := encryptIDNo(testPIISecret, "110101199001011234", "user-abc")
	if _, err := decryptIDNo("a-different-secret", blob, "user-abc"); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestDecryptIDNo_EmptyIsNotEncrypted(t *testing.T) {
	if _, err := decryptIDNo(testPIISecret, nil, "u"); err != ErrIDNoNotEncrypted {
		t.Fatalf("empty blob: got %v, want ErrIDNoNotEncrypted", err)
	}
}

func TestEncKeyIndependentOfBlindIndex(t *testing.T) {
	// The AES key derivation must not equal the raw piiSecret bytes used by the
	// HMAC blind index — domain separation.
	k := deriveIDNoKey(testPIISecret)
	if string(k[:]) == testPIISecret {
		t.Fatal("derived key equals piiSecret")
	}
}
