package quality

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Format validates encoding and, for json/csv, parseability.
func Format(content []byte, contentType string) Check {
	report := map[string]any{"content_type": contentType}
	if !utf8.Valid(content) {
		report["error"] = "content is not valid UTF-8"
		return Check{Type: TypeFormat, Result: ResultFail, Report: report}
	}
	switch {
	case strings.Contains(contentType, "json"):
		if !json.Valid(content) {
			report["error"] = "invalid JSON"
			return Check{Type: TypeFormat, Result: ResultFail, Report: report}
		}
	case strings.Contains(contentType, "csv"):
		if _, err := csv.NewReader(strings.NewReader(string(content))).ReadAll(); err != nil {
			report["error"] = "invalid CSV: " + err.Error()
			return Check{Type: TypeFormat, Result: ResultFail, Report: report}
		}
	}
	report["encoding"] = "utf-8"
	return Check{Type: TypeFormat, Result: ResultPass, Report: report}
}

// Stats computes basic size/line statistics. SampleCount (non-empty lines) is
// returned separately so the caller can persist it on the dataset.
func Stats(content []byte) (Check, int64) {
	lines := strings.Split(string(content), "\n")
	var nonEmpty int64
	var totalLen int
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			nonEmpty++
		}
		totalLen += len(ln)
	}
	avg := 0
	if len(lines) > 0 {
		avg = totalLen / len(lines)
	}
	report := map[string]any{
		"bytes":           len(content),
		"lines":           len(lines),
		"non_empty_lines": nonEmpty,
		"avg_line_bytes":  avg,
		"runes":           utf8.RuneCount(content),
	}
	result := ResultPass
	if nonEmpty == 0 {
		result = ResultWarn // empty/whitespace-only content
	}
	return Check{Type: TypeStats, Result: result, Report: report}, nonEmpty
}
