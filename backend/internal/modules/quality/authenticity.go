package quality

import (
	"bytes"
	"encoding/csv"
	"math"
	"strconv"
	"strings"
)

// Tunable thresholds (production moves these to config — docs §6). Conservative
// by design: this is the always-on Go baseline; the PaperGuard sidecar adds the
// deeper detectors (GRIM/SPRITE/...). Signal, not verdict.
const (
	authMinColN     = 30   // min numeric values for a column to be screened
	authFDRAlpha    = 0.05 // significance after Benjamini-Hochberg
	benfordMinRatio = 100  // max/min span required for Benford to be meaningful
	sentinelMinMode = 0.40 // a min/max value taking ≥40% of a column is sentinel-like
	dupRowsMinRatio = 0.30 // duplicate-row fraction that starts to matter
)

// authFinding is one statistical signal on one column. Every finding carries an
// academic reference and ≥3 innocent explanations — it invites a closer look,
// it is never a verdict (mirrors PaperGuard's epistemic stance).
type authFinding struct {
	Detector             string   `json:"detector"`
	Column               string   `json:"column"`
	Reference            string   `json:"reference"`
	Statistic            float64  `json:"statistic,omitempty"`
	PValue               float64  `json:"p_value,omitempty"`
	PValueAdjusted       float64  `json:"p_value_adjusted,omitempty"`
	Severity             string   `json:"severity"`
	Significant          bool     `json:"significant"`
	InnocentExplanations []string `json:"innocent_explanations"`
}

// Authenticity screens a tabular (CSV) payload for statistical signs of
// fabricated/synthetic/templated numeric data and returns one aggregate Check
// (Type=authenticity) whose report holds a 0-100 score, a band, and the
// findings. It never returns fail — authenticity is advisory and must not bounce
// a dataset; result is pass (clean) or warn (review/suspect).
func Authenticity(content []byte, contentType string) Check {
	if !strings.Contains(contentType, "csv") {
		return authCheck(100, "clean", 0, 0, nil, map[string]any{"applicable": false, "note": "non-CSV payload — handled by the statistical sidecar / skipped"})
	}
	cols, nrows, ok := parseNumericColumns(content)
	if !ok || len(cols) == 0 {
		return authCheck(100, "clean", 0, 0, nil, map[string]any{"applicable": false, "note": "no screenable numeric columns (too few rows or non-numeric)"})
	}

	var findings []authFinding
	for _, c := range cols {
		if f, ok := benfordFinding(c); ok {
			findings = append(findings, f)
		}
		if f, ok := terminalDigitFinding(c); ok {
			findings = append(findings, f)
		}
		if f, ok := sentinelFinding(c); ok {
			findings = append(findings, f)
		}
	}
	if f, ok := duplicateRowsFinding(content, nrows); ok {
		findings = append(findings, f)
	}

	applyFDR(findings)
	score, band := scoreAuthenticity(findings)
	return authCheck(score, band, len(findings), len(cols), findings, map[string]any{"applicable": true, "rows": nrows})
}

func authCheck(score int, band string, nFindings, nCols int, findings []authFinding, extra map[string]any) Check {
	report := map[string]any{
		"score":            score,
		"band":             band,
		"n_findings":       nFindings,
		"columns_screened": nCols,
		"findings":         findings,
	}
	for k, v := range extra {
		report[k] = v
	}
	result := ResultPass
	if band != "clean" {
		result = ResultWarn // review/suspect surface a signal; never auto-fail
	}
	return Check{Type: TypeAuthenticity, Result: result, Report: report}
}

// numColumn is one parsed numeric column.
type numColumn struct {
	name   string
	values []float64
}

// parseNumericColumns parses CSV, detects a header heuristically, and returns
// the columns with enough numeric values to screen.
func parseNumericColumns(content []byte) ([]numColumn, int, bool) {
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	rows, err := r.ReadAll()
	if err != nil || len(rows) < authMinColN {
		return nil, 0, false
	}

	header := !allNumeric(rows[0])
	start := 0
	if header {
		start = 1
	}
	ncol := 0
	for _, row := range rows {
		if len(row) > ncol {
			ncol = len(row)
		}
	}

	cols := make([]numColumn, 0, ncol)
	for j := 0; j < ncol; j++ {
		var vals []float64
		nonEmpty := 0
		for i := start; i < len(rows); i++ {
			if j >= len(rows[i]) {
				continue
			}
			cell := strings.TrimSpace(rows[i][j])
			if cell == "" {
				continue
			}
			nonEmpty++
			if f, err := strconv.ParseFloat(cell, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				vals = append(vals, f)
			}
		}
		// A column is numeric if most non-empty cells parse and there are enough.
		if len(vals) >= authMinColN && nonEmpty > 0 && float64(len(vals))/float64(nonEmpty) >= 0.8 {
			name := "col_" + strconv.Itoa(j)
			if header && j < len(rows[0]) && strings.TrimSpace(rows[0][j]) != "" {
				name = strings.TrimSpace(rows[0][j])
			}
			cols = append(cols, numColumn{name: name, values: vals})
		}
	}
	return cols, len(rows) - start, true
}

func allNumeric(row []string) bool {
	any := false
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		any = true
		if _, err := strconv.ParseFloat(c, 64); err != nil {
			return false
		}
	}
	return any
}

// benfordFinding runs a first-digit chi-square against Benford's law, but only
// on positive columns spanning ≥2 orders of magnitude (where Benford applies).
func benfordFinding(c numColumn) (authFinding, bool) {
	minV, maxV := math.Inf(1), math.Inf(-1)
	var counts [10]int
	n := 0
	for _, v := range c.values {
		if v <= 0 {
			return authFinding{}, false // Benford needs positive, multi-scale data
		}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		counts[firstDigit(v)]++
		n++
	}
	if n < authMinColN || minV <= 0 || maxV/minV < benfordMinRatio {
		return authFinding{}, false
	}
	chi2 := 0.0
	for d := 1; d <= 9; d++ {
		exp := float64(n) * math.Log10(1+1/float64(d))
		diff := float64(counts[d]) - exp
		chi2 += diff * diff / exp
	}
	p := chiSquareP(chi2, 8)
	return authFinding{
		Detector:  "benford_first_digit",
		Column:    c.name,
		Reference: "Benford 1938; Nigrini 2012",
		Statistic: round2(chi2),
		PValue:    p,
		InnocentExplanations: []string{
			"the column is naturally bounded or single-scale (Benford does not apply)",
			"values are derived (ratios, percentages, indices) rather than raw counts",
			"a unit or rounding convention concentrates leading digits",
		},
	}, true
}

// terminalDigitFinding runs a last-digit uniformity chi-square on integer-like
// columns (Mosimann 1995; the Geng last-digit test, 2025) — fabricated data
// often over-uses round or preferred final digits.
func terminalDigitFinding(c numColumn) (authFinding, bool) {
	integerish := 0
	for _, v := range c.values {
		if math.Abs(v-math.Round(v)) < 1e-9 {
			integerish++
		}
	}
	if len(c.values) < authMinColN || float64(integerish)/float64(len(c.values)) < 0.9 {
		return authFinding{}, false
	}
	var counts [10]int
	for _, v := range c.values {
		d := int(math.Mod(math.Abs(math.Round(v)), 10))
		counts[d]++
	}
	n := len(c.values)
	exp := float64(n) / 10
	chi2 := 0.0
	for d := 0; d < 10; d++ {
		diff := float64(counts[d]) - exp
		chi2 += diff * diff / exp
	}
	p := chiSquareP(chi2, 9)
	return authFinding{
		Detector:  "terminal_digit_uniformity",
		Column:    c.name,
		Reference: "Mosimann 1995; Geng 2025",
		Statistic: round2(chi2),
		PValue:    p,
		InnocentExplanations: []string{
			"the quantity is genuinely measured to a coarse precision (ends in 0/5)",
			"values come from a rounding or pricing convention",
			"the column encodes categories or codes, not measurements",
		},
	}, true
}

// sentinelFinding flags a column where the min or max value is over-represented
// (≥40%) amid otherwise varied data — the classic "-999 / 9999 placeholder mixed
// into real values" pattern.
func sentinelFinding(c numColumn) (authFinding, bool) {
	freq := map[float64]int{}
	for _, v := range c.values {
		freq[v]++
	}
	if len(freq) <= 5 { // low-cardinality columns are likely flags/labels, not measurements
		return authFinding{}, false
	}
	minV, maxV := c.values[0], c.values[0]
	for _, v := range c.values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	n := len(c.values)
	for _, cand := range []float64{minV, maxV} {
		share := float64(freq[cand]) / float64(n)
		if share >= sentinelMinMode {
			return authFinding{
				Detector:    "sentinel_value",
				Column:      c.name,
				Reference:   "data-quality placeholder/sentinel screening",
				Statistic:   round2(share),
				Significant: true,
				Severity:    "medium",
				InnocentExplanations: []string{
					"the value is a legitimate floor/cap (e.g. a clipped sensor range)",
					"the column is genuinely zero-inflated or censored",
					"the repeated extreme reflects a real, common observation",
				},
			}, true
		}
	}
	return authFinding{}, false
}

// duplicateRowsFinding flags a high fraction of exact-duplicate rows — a sign of
// padded, copy-pasted, or templated data.
func duplicateRowsFinding(content []byte, nrows int) (authFinding, bool) {
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) < authMinColN {
		return authFinding{}, false
	}
	seen := map[string]struct{}{}
	total := 0
	for i := 0; i < len(rows); i++ {
		key := strings.Join(rows[i], "\x00")
		total++
		seen[key] = struct{}{}
	}
	if total == 0 {
		return authFinding{}, false
	}
	dupRatio := float64(total-len(seen)) / float64(total)
	if dupRatio < dupRowsMinRatio {
		return authFinding{}, false
	}
	sev := "low"
	switch {
	case dupRatio > 0.6:
		sev = "high"
	case dupRatio > 0.4:
		sev = "medium"
	}
	return authFinding{
		Detector:    "duplicate_rows",
		Column:      "(row-level)",
		Reference:   "exact-duplicate row analysis",
		Statistic:   round2(dupRatio),
		Significant: true,
		Severity:    sev,
		InnocentExplanations: []string{
			"the schema legitimately has few distinct combinations (categorical/log data)",
			"duplicates are valid repeated events or measurements",
			"a wide export naturally repeats key columns",
		},
	}, true
}

// applyFDR fills PValueAdjusted/Significant/Severity for p-value findings using
// Benjamini-Hochberg across all of them; ratio-based findings already set theirs.
func applyFDR(findings []authFinding) {
	idx := make([]int, 0, len(findings))
	ps := make([]float64, 0, len(findings))
	for i := range findings {
		// Only the chi-square detectors carry a meaningful raw p-value; the
		// ratio detectors (sentinel/duplicate) set their own severity already.
		if findings[i].Detector == "benford_first_digit" || findings[i].Detector == "terminal_digit_uniformity" {
			idx = append(idx, i)
			ps = append(ps, findings[i].PValue)
		}
	}
	adj := benjaminiHochberg(ps)
	for k, i := range idx {
		findings[i].PValueAdjusted = adj[k]
		findings[i].Significant = adj[k] < authFDRAlpha
		findings[i].Severity = severityFromP(adj[k], findings[i].Significant)
	}
}

func severityFromP(p float64, significant bool) string {
	switch {
	case !significant:
		return "info"
	case p < 0.001:
		return "high"
	case p < 0.01:
		return "medium"
	default:
		return "low"
	}
}

// scoreAuthenticity turns significant findings into a 0-100 score and a band.
func scoreAuthenticity(findings []authFinding) (int, string) {
	weight := map[string]int{"info": 0, "low": 4, "medium": 10, "high": 20}
	score := 100
	for _, f := range findings {
		if f.Significant {
			score -= weight[f.Severity]
		}
	}
	if score < 0 {
		score = 0
	}
	switch {
	case score >= 85:
		return score, "clean"
	case score >= 60:
		return score, "review"
	default:
		return score, "suspect"
	}
}

func firstDigit(f float64) int {
	f = math.Abs(f)
	if f == 0 {
		return 0
	}
	for f >= 10 {
		f /= 10
	}
	for f < 1 {
		f *= 10
	}
	return int(f)
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
