package quality

import (
	"fmt"
	"hash/fnv"
	"math/bits"
	"strconv"
	"unicode"
)

// SimHash returns a 64-bit SimHash (as 16 hex chars) over rune-bigram shingles.
// Bigrams of (whitespace-stripped) runes work for Chinese, which has no word
// boundaries. Near-duplicate content yields a small Hamming distance — used to
// flag re-uploaded/resold data (docs §6.3).
func SimHash(content []byte) string {
	shingles := bigramShingles(content)
	if len(shingles) == 0 {
		// Empty / whitespace-only content has no shingles. Return "" (the caller's
		// NULLIF stores NULL) rather than the all-zero fingerprint, which would
		// collide as a "near-duplicate" with every other degenerate upload.
		return ""
	}
	var vec [64]int
	for _, sh := range shingles {
		h := fnvHash(sh)
		for i := 0; i < 64; i++ {
			if h&(uint64(1)<<uint(i)) != 0 {
				vec[i]++
			} else {
				vec[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if vec[i] > 0 {
			out |= uint64(1) << uint(i)
		}
	}
	return fmt.Sprintf("%016x", out)
}

// Hamming returns the bit distance between two SimHash hex strings; -1 if either
// is unparseable. Smaller = more similar (0 = identical fingerprint).
func Hamming(a, b string) int {
	x, err1 := strconv.ParseUint(a, 16, 64)
	y, err2 := strconv.ParseUint(b, 16, 64)
	if err1 != nil || err2 != nil {
		return -1
	}
	return bits.OnesCount64(x ^ y)
}

func bigramShingles(content []byte) []string {
	clean := make([]rune, 0, len(content))
	for _, r := range string(content) {
		if !unicode.IsSpace(r) {
			clean = append(clean, r)
		}
	}
	if len(clean) < 2 {
		if len(clean) == 1 {
			return []string{string(clean)}
		}
		return nil
	}
	out := make([]string, 0, len(clean)-1)
	for i := 0; i+1 < len(clean); i++ {
		out = append(out, string(clean[i:i+2]))
	}
	return out
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
