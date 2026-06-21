package quality

import "testing"

func TestSimHash_DegenerateContentReturnsEmpty(t *testing.T) {
	// Empty / whitespace-only content has no shingles. It must hash to "" (so the
	// caller's NULLIF stores NULL) — NOT the all-zero fingerprint "0000000000000000",
	// which would collide as a "near-duplicate" with every other degenerate upload.
	for _, s := range []string{"", "   ", "\n\t  \n", " "} {
		if got := SimHash([]byte(s)); got != "" {
			t.Fatalf("degenerate content %q should hash to empty, got %q", s, got)
		}
	}
}

func TestSimHash_RealContentAndNearDup(t *testing.T) {
	a := SimHash([]byte("the quick brown fox jumps over the lazy dog"))
	if len(a) != 16 {
		t.Fatalf("real content should produce a 16-hex fingerprint, got %q", a)
	}
	near := SimHash([]byte("the quick brown fox jumps over the lazy cat")) // one word changed
	far := SimHash([]byte("a completely unrelated sentence about databases"))
	dn, df := Hamming(a, near), Hamming(a, far)
	if dn < 0 || dn > 16 {
		t.Fatalf("near-duplicate Hamming distance should be small, got %d", dn)
	}
	if dn >= df {
		t.Fatalf("a near-duplicate (%d) should be closer than an unrelated doc (%d)", dn, df)
	}
}

func TestSimHash_CJKBigramsStable(t *testing.T) {
	// CJK has no word boundaries; rune-bigram shingles must still yield a stable,
	// non-empty fingerprint, and identical content the same fingerprint.
	h1 := SimHash([]byte("可信数据市场科研滩头"))
	h2 := SimHash([]byte("可信数据市场科研滩头"))
	if len(h1) != 16 || h1 != h2 {
		t.Fatalf("CJK content should hash deterministically to 16 hex, got %q / %q", h1, h2)
	}
}

func TestHamming_EmptyFingerprintNeverMatches(t *testing.T) {
	// A degenerate ("") fingerprint must not be treated as a 0-distance match.
	if Hamming("", "abc") != -1 {
		t.Fatal("unparseable fingerprint should yield -1")
	}
	if Hamming(SimHash([]byte("")), SimHash([]byte("  "))) != -1 {
		t.Fatal("two empty fingerprints must yield -1, never a 0-distance dup match")
	}
}
