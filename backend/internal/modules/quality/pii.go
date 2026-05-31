package quality

import "regexp"

// PII detectors for common Chinese personal data (docs §2.2 / §6.3). These are
// heuristics — high recall, some false positives — enough to red-flag a dataset
// for de-identification. Order matters: more specific patterns first.
var piiPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"id_card", regexp.MustCompile(`(?:^|[^0-9])(\d{17}[\dXx])(?:[^0-9Xx]|$)`)}, // 18-digit PRC ID
	{"phone", regexp.MustCompile(`(?:^|[^0-9])(1[3-9]\d{9})(?:[^0-9]|$)`)},      // mainland mobile
	{"bank_card", regexp.MustCompile(`(?:^|[^0-9])(\d{16,19})(?:[^0-9]|$)`)},    // bank card (broad)
	{"email", regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)},
	{"ipv4", regexp.MustCompile(`(?:^|[^0-9])((?:\d{1,3}\.){3}\d{1,3})(?:[^0-9]|$)`)},
}

// PII scans content for personal data. declaredPII reflects the seller's source
// declaration: if they declared NO personal info but we find some, that is a
// hard fail (the declaration is false); if they declared PII, it is a warning
// (they must still de-identify, but it was disclosed).
func PII(content []byte, declaredPII bool) Check {
	text := string(content)
	counts := map[string]int{}
	total := 0
	for _, p := range piiPatterns {
		n := len(p.re.FindAllString(text, -1))
		if n > 0 {
			counts[p.name] = n
			total += n
		}
	}
	report := map[string]any{"matches": counts, "total": total, "declared_pii": declaredPII}
	switch {
	case total == 0:
		return Check{Type: TypePII, Result: ResultPass, Report: report}
	case declaredPII:
		// Disclosed — warn, still expected to de-identify before publishing.
		return Check{Type: TypePII, Result: ResultWarn, Report: report}
	default:
		report["error"] = "personal information detected but source declaration says none"
		return Check{Type: TypePII, Result: ResultFail, Report: report}
	}
}
