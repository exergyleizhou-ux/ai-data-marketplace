package quality

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// piiMask is the token substituted for detected personal data. It contains no
// digits and no "@", so re-scanning redacted content yields zero residual — the
// invariant PIIRedaction enforces.
const piiMask = "***"

// Detector confidence. Validated detectors (checksum / Luhn / range) are
// high-precision; heuristic ones trade precision for recall and are reported as
// such so buyers can weigh them (mirrors PaperGuard's epistemic framing — every
// signal is qualified, never a verdict).
const (
	confHigh      = "high"
	confHeuristic = "heuristic"
)

// detector matches one class of personal data. The regex captures the bare
// token (no boundary groups), so detection, masking, and post-redaction
// verification all operate on the exact same span — this keeps masking from
// eating flanking characters and from missing adjacent matches.
type detector struct {
	name       string
	confidence string
	re         *regexp.Regexp
	flank      func(rune) bool   // reject a hit if an adjacent rune satisfies this (nil = no rule)
	validate   func(string) bool // reject a hit if this returns false (nil = accept any)
}

func isDigit(r rune) bool      { return r >= '0' && r <= '9' }
func isDigitOrX(r rune) bool   { return isDigit(r) || r == 'X' || r == 'x' }
func isDigitOrDot(r rune) bool { return isDigit(r) || r == '.' }
func isAlnum(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// detectors for common Chinese personal data (docs §2.2 / §6.3; supports the
// ToS §5.1 de-identification warranty). Order matters: on overlap, earlier
// (more specific) detectors win. Numeric detectors use flank rules instead of
// regex boundaries so adjacent values are all caught.
var detectors = []detector{
	{"id_card", confHigh, regexp.MustCompile(`\d{17}[\dXx]`), isDigitOrX, validIDCard},
	{"bank_card", confHigh, regexp.MustCompile(`\d{16,19}`), isDigit, validLuhn},
	{"phone", confHigh, regexp.MustCompile(`1[3-9]\d{9}`), isDigit, nil},
	{"email", confHigh, regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`), nil, nil},
	{"ipv4", confHigh, regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3}`), isDigitOrDot, validIPv4},
	{"passport", confHeuristic, regexp.MustCompile(`[EeGgDdSsPpHh]\d{8}`), isAlnum, nil},
	{"plate", confHeuristic, regexp.MustCompile(`[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-HJ-NP-Z][A-HJ-NP-Z0-9]{4}[A-HJ-NP-Z0-9挂学警港澳]`), nil, nil},
	{"gps", confHeuristic, regexp.MustCompile(`\d{1,3}\.\d{4,}\s*,\s*\d{1,3}\.\d{4,}`), nil, nil},
	{"address", confHeuristic, regexp.MustCompile(`[\x{4e00}-\x{9fa5}]{2,6}(?:市|区|县)[\x{4e00}-\x{9fa5}]{0,10}?(?:路|街|大道|巷|弄)\d+号?(?:\d+室)?`), nil, nil},
}

// match is one accepted PII span in the source string.
type match struct {
	name       string
	confidence string
	start, end int
}

// scan returns the non-overlapping, validated PII spans in s, sorted by
// position. It is the single source of truth shared by PII, MaskPII, and
// PIIRedaction.
func scan(s string) []match {
	var all []match
	for _, d := range detectors {
		for _, loc := range d.re.FindAllStringIndex(s, -1) {
			st, en := loc[0], loc[1]
			if d.flank != nil && isFlanked(s, st, en, d.flank) {
				continue
			}
			if d.validate != nil && !d.validate(s[st:en]) {
				continue
			}
			all = append(all, match{d.name, d.confidence, st, en})
		}
	}
	return resolveOverlaps(all)
}

// isFlanked reports whether the rune immediately before start or after end
// satisfies excl — used to reject a numeric hit that is part of a longer run
// (e.g. an 11-digit phone inside a 20-digit blob).
func isFlanked(s string, start, end int, excl func(rune) bool) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); excl(r) {
			return true
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); excl(r) {
			return true
		}
	}
	return false
}

// resolveOverlaps keeps the longest (then earliest-declared) span when matches
// overlap, so a single value is never counted or masked twice.
func resolveOverlaps(ms []match) []match {
	if len(ms) < 2 {
		return ms
	}
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].start != ms[j].start {
			return ms[i].start < ms[j].start
		}
		return (ms[i].end - ms[i].start) > (ms[j].end - ms[j].start) // longer first
	})
	out := ms[:0:0]
	watermark := -1
	for _, m := range ms {
		if m.start >= watermark {
			out = append(out, m)
			watermark = m.end
		}
	}
	return out
}

// maskSpans rewrites s, replacing each (sorted, non-overlapping) span with the
// mask token. Only the PII span is replaced — flanking text is preserved.
func maskSpans(s string, ms []match) string {
	if len(ms) == 0 {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, m := range ms {
		if m.start < prev {
			continue // defensive; resolveOverlaps already guarantees this
		}
		b.WriteString(s[prev:m.start])
		b.WriteString(piiMask)
		prev = m.end
	}
	b.WriteString(s[prev:])
	return b.String()
}

// MaskPII replaces detected personal data with "***" so content can be shown in
// previews without leaking PII. Defense-in-depth: published data should already
// be clean, but previews are public/semi-public.
func MaskPII(s string) string {
	return maskSpans(s, scan(s))
}

// PII scans content for personal data. declaredPII reflects the seller's source
// declaration: if they declared NO personal info but we find some, that is a
// hard fail (the declaration is false); if they declared PII, it is a warning
// (they must still de-identify, but it was disclosed).
func PII(content []byte, declaredPII bool) Check {
	matches := scan(string(content))
	counts := map[string]int{}
	byConfidence := map[string]int{}
	for _, m := range matches {
		counts[m.name]++
		byConfidence[m.confidence]++
	}
	report := map[string]any{
		"matches":       counts,
		"by_confidence": byConfidence,
		"total":         len(matches),
		"declared_pii":  declaredPII,
	}
	switch {
	case len(matches) == 0:
		return Check{Type: TypePII, Result: ResultPass, Report: report}
	case declaredPII:
		// Disclosed — warn, still expected to de-identify before publishing.
		return Check{Type: TypePII, Result: ResultWarn, Report: report}
	default:
		report["error"] = "personal information detected but source declaration says none"
		return Check{Type: TypePII, Result: ResultFail, Report: report}
	}
}

// PIIRedaction proves that de-identification leaves zero residual: it detects
// PII, applies MaskPII, then re-scans the masked output and asserts nothing
// remains. The passing row is the buyer-facing trust artifact behind the ToS
// §5.1 "necessary de-identification has been performed" warranty; a failing row
// means the masker is incomplete relative to detection (a regression guard).
func PIIRedaction(content []byte) Check {
	original := scan(string(content))
	report := map[string]any{"detected_total": len(original)}
	if len(original) == 0 {
		report["verified"] = true
		report["note"] = "no personal information to redact"
		return Check{Type: TypePIIRedaction, Result: ResultPass, Report: report}
	}
	residual := scan(maskSpans(string(content), original))
	report["residual_total"] = len(residual)
	if len(residual) > 0 {
		residualTypes := map[string]int{}
		for _, m := range residual {
			residualTypes[m.name]++
		}
		report["residual_types"] = residualTypes
		report["verified"] = false
		report["error"] = "personal information remains after de-identification"
		return Check{Type: TypePIIRedaction, Result: ResultFail, Report: report}
	}
	report["verified"] = true
	return Check{Type: TypePIIRedaction, Result: ResultPass, Report: report}
}

// validLuhn reports whether an all-digit string passes the Luhn checksum — used
// to keep the broad bank-card pattern from flagging arbitrary long numbers.
func validLuhn(s string) bool {
	if len(s) == 0 {
		return false
	}
	sum, alt := 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if alt {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// validIDCard verifies the PRC resident ID check digit (GB 11643-1999,
// weighted mod-11). Cuts false positives from arbitrary 18-char digit strings.
func validIDCard(s string) bool {
	if len(s) != 18 {
		return false
	}
	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checks := [11]byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		sum += int(c-'0') * weights[i]
	}
	last := s[17]
	if last == 'x' {
		last = 'X'
	}
	return checks[sum%11] == last
}

// validIPv4 reports whether each dotted octet is in 0..255.
func validIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
