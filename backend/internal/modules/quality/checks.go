package quality

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
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
	case strings.Contains(contentType, "ndjson") || strings.Contains(contentType, "jsonl"):
		// JSON Lines: each non-empty line must be valid JSON (checked before the
		// generic "json" case, which "ndjson" would otherwise match and fail).
		sc := bufio.NewScanner(bytes.NewReader(content))
		sc.Buffer(make([]byte, 0, 64*1024), 32<<20) // tolerate long records
		n := 0
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			n++
			if !json.Valid(line) {
				report["error"] = fmt.Sprintf("invalid JSON on line %d", n)
				return Check{Type: TypeFormat, Result: ResultFail, Report: report}
			}
		}
		if err := sc.Err(); err != nil {
			report["error"] = "read error: " + err.Error()
			return Check{Type: TypeFormat, Result: ResultFail, Report: report}
		}
		report["jsonl_records"] = n
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
	case strings.Contains(contentType, "tab-separated") || strings.Contains(contentType, "tsv"):
		r := csv.NewReader(strings.NewReader(string(content)))
		r.Comma = '\t'
		r.FieldsPerRecord = -1
		if _, err := r.ReadAll(); err != nil {
			report["error"] = "invalid TSV: " + err.Error()
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
