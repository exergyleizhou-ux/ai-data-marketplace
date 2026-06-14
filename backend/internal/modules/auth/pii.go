package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// PII field encryption for the raw ID number (身份证号).
//
// Two columns work together:
//   - id_no_hash       — HMAC(piiSecret, idNo): a BLIND INDEX for equality/dedup,
//     never reversible (unchanged from before).
//   - id_no_ciphertext — AES-256-GCM(idNo, AAD=user_id): reversible at-rest
//     encryption so the plaintext can be retrieved for lawful disclosure
//     (ops-only, audited).
//
// The AES key is derived from piiSecret with domain separation, so the encryption
// key and the blind-index HMAC key are cryptographically independent even though
// they share one master secret.
//
// Production hardening (not blocking): source a dedicated key from a KMS / secret
// manager (envelope encryption — KMS-wrapped data key) instead of deriving from
// piiSecret. The leading version byte lets you rotate to that without re-reading
// existing rows: bump piiKeyVersion, decrypt-old/encrypt-new lazily on access.
const piiKeyVersion = 1

// ErrIDNoNotEncrypted means the record has no stored ciphertext (company KYC or
// a row created before this feature) — there is no plaintext to reveal.
var ErrIDNoNotEncrypted = errors.New("id number not available for retrieval")

func deriveIDNoKey(piiSecret string) [32]byte {
	// Distinct label so this key never collides with the HMAC blind-index key.
	return sha256.Sum256([]byte("ai-data-marketplace/pii-aes/v1\x00" + piiSecret))
}

func newIDNoGCM(piiSecret string) (cipher.AEAD, error) {
	key := deriveIDNoKey(piiSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("pii cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// encryptIDNo returns version(1) ‖ nonce(12) ‖ GCM-ciphertext. AAD = userID binds
// the blob to its owning record, so a ciphertext cannot be moved to another row.
func encryptIDNo(piiSecret, idNo, userID string) ([]byte, error) {
	gcm, err := newIDNoGCM(piiSecret)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("pii nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(idNo), []byte(userID))
	out := make([]byte, 0, 1+len(nonce)+len(ct))
	out = append(out, piiKeyVersion)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// decryptIDNo reverses encryptIDNo. It fails (authentication error) if the blob,
// userID (AAD) or key don't all match — tamper / wrong-record / wrong-key.
func decryptIDNo(piiSecret string, blob []byte, userID string) (string, error) {
	if len(blob) == 0 {
		return "", ErrIDNoNotEncrypted
	}
	if blob[0] != piiKeyVersion {
		return "", fmt.Errorf("pii: unsupported key version %d", blob[0])
	}
	gcm, err := newIDNoGCM(piiSecret)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(blob) < 1+ns {
		return "", errors.New("pii: ciphertext too short")
	}
	nonce, ct := blob[1:1+ns], blob[1+ns:]
	pt, err := gcm.Open(nil, nonce, ct, []byte(userID))
	if err != nil {
		return "", fmt.Errorf("pii: decrypt/authenticate: %w", err)
	}
	return string(pt), nil
}
