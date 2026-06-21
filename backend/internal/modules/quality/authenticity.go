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
	authMinColN      = 30   // min numeric values for a column to be screened
	authFDRAlpha     = 0.05 // significance after Benjamini-Hochberg
	benfordMinRatio  = 100  // max/min span required for Benford to be meaningful
	benfordMinLogStd = 0.5  // min std-dev of log10(values): below this the column is
	//                        sequential/uniform/binned (not log-uniform) and Benford
	//                        does not apply (real Benford data is ≳0.8; sequential ≲0.45)
	sentinelMinMode  = 0.40 // a min/max value taking ≥40% of a column is sentinel-like
	sentinelOutlierK = 3.0  // the repeated extreme must sit ≥K·std from the nearest
	//                         other value to count as an out-of-band sentinel (vs a
	//                         contiguous, legitimately zero-inflated/censored value)
	dupRowsMinRatio = 0.30 // duplicate-row fraction that starts to matter

	// Applicability gates for the terminal-digit test: below these the column is
	// categorical/coded, not a measurement, and the uniform-last-digit null is invalid.
	terminalDigitMinDistinct = 20  // ≥20 distinct values (excludes binary/Likert/codes)
	terminalDigitMinSpan     = 100 // value range ≥100 (excludes small-range counts)
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
	delim, ok := tabularDelimiter(contentType)
	if !ok {
		note := "non-tabular payload — statistical screening skipped"
		if IsParquet(contentType) {
			// Binary columnar — the Go baseline can't read it; the PaperGuard
			// sidecar (pandas) screens it when configured.
			note = "Parquet — statistical screening runs in the PaperGuard sidecar"
		}
		return authCheck(100, "clean", 0, 0, nil, map[string]any{"applicable": false, "note": note})
	}
	cols, nrows, ok := parseNumericColumns(content, delim)
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
	if f, ok := duplicateRowsFinding(content, nrows, delim); ok {
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

// IsTabular reports whether a content type is a delimited table (CSV/TSV) the
// in-process Go baseline can screen.
func IsTabular(contentType string) bool {
	_, ok := tabularDelimiter(contentType)
	return ok
}

// IsParquet reports whether a content type is Apache Parquet.
func IsParquet(contentType string) bool {
	return strings.Contains(contentType, "parquet")
}

// IsScreenable reports whether the authenticity pipeline (Go baseline OR the
// PaperGuard sidecar) can screen this content type — CSV/TSV in-process, Parquet
// via the sidecar. Used by the worker to decide what to send to the sidecar.
func IsScreenable(contentType string) bool {
	return IsTabular(contentType) || IsParquet(contentType)
}

// tabularDelimiter maps a content type to its column delimiter. CSV uses comma,
// TSV uses tab; anything else is non-tabular (screening not applicable).
func tabularDelimiter(contentType string) (rune, bool) {
	switch {
	case strings.Contains(contentType, "tab-separated") || strings.Contains(contentType, "tsv"):
		return '\t', true
	case strings.Contains(contentType, "csv"):
		return ',', true
	default:
		return 0, false
	}
}

// parseNumericColumns parses a delimited table, detects a header heuristically,
// and returns the columns with enough numeric values to screen.
func parseNumericColumns(content []byte, delim rune) ([]numColumn, int, bool) {
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = delim
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
	// Distributional applicability gate. Span ≥100 is necessary but NOT sufficient
	// for Benford: sequential IDs, evenly-binned, and uniform-over-a-range columns
	// span orders of magnitude yet are not log-uniform and would false-flag. Benford
	// holds for log-uniform (multiplicative) data; require enough spread in log10
	// space to distinguish it (real Benford data ≳0.8; sequential/uniform ≲0.45).
	var sl, sl2 float64
	for _, v := range c.values {
		l := math.Log10(v)
		sl += l
		sl2 += l * l
	}
	nf := float64(n)
	logStd := math.Sqrt(math.Max(sl2/nf-(sl/nf)*(sl/nf), 0))
	if logStd < benfordMinLogStd {
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
	// Applicability gate: the last-digit-uniformity null (a uniform terminal digit)
	// only holds for high-cardinality integers that span enough magnitude. Small-
	// range or low-cardinality integer columns are categorical/coded (binary flags,
	// Likert scales, day-of-week, month, status codes) where the digit is inherently
	// non-uniform — running the test there produces false "suspect" flags on
	// legitimate data. Require both a cardinality floor and a magnitude span.
	distinct := map[float64]struct{}{}
	minV, maxV := c.values[0], c.values[0]
	for _, v := range c.values {
		distinct[v] = struct{}{}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if len(distinct) < terminalDigitMinDistinct || maxV-minV < terminalDigitMinSpan {
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
		if share < sentinelMinMode {
			continue
		}
		// A real sentinel (-999 / 9999) is a distributional OUTLIER — far from the
		// rest of the data. A legitimately zero-inflated/censored column's repeated
		// extreme is CONTIGUOUS with the data (0 next to 1,2,3…). Only flag when the
		// candidate sits far (≥K·std) from the nearest other value; otherwise it's
		// ordinary mode-at-the-boundary, a false positive on count/zero-inflated data.
		nearest := math.Inf(1)
		var sum, sum2 float64
		m := 0
		for _, v := range c.values {
			if v == cand {
				continue
			}
			if d := math.Abs(v - cand); d < nearest {
				nearest = d
			}
			sum += v
			sum2 += v * v
			m++
		}
		if m == 0 {
			continue
		}
		mf := float64(m)
		std := math.Sqrt(math.Max(sum2/mf-(sum/mf)*(sum/mf), 0))
		if std <= 0 || nearest < sentinelOutlierK*std {
			continue // contiguous with the data → zero-inflation, not a sentinel
		}
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
	return authFinding{}, false
}

// duplicateRowsFinding flags a high fraction of exact-duplicate rows — a sign of
// padded, copy-pasted, or templated data.
func duplicateRowsFinding(content []byte, nrows int, delim rune) (authFinding, bool) {
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = delim
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return authFinding{}, false
	}
	// Exclude the header row (same heuristic as parseNumericColumns): it is always
	// unique, so counting it dilutes the duplicate ratio across the detection
	// threshold and reports a statistic that is wrong by one row.
	start := 0
	if len(rows) > 0 && !allNumeric(rows[0]) {
		start = 1
	}
	data := rows[start:]
	if len(data) < authMinColN {
		return authFinding{}, false
	}
	seen := map[string]struct{}{}
	for i := range data {
		seen[strings.Join(data[i], "\x00")] = struct{}{}
	}
	total := len(data)
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
	// Cap the penalty at the single strongest finding PER COLUMN. Several detectors
	// keying off the same column property (Benford + terminal-digit, or sentinel +
	// duplicate-rows) are correlated, not independent evidence; summing them double-
	// counts one quirk and unfairly pushes legitimate data into "review"/"suspect".
	perCol := map[string]int{}
	for _, f := range findings {
		if f.Significant {
			if w := weight[f.Severity]; w > perCol[f.Column] {
				perCol[f.Column] = w
			}
		}
	}
	score := 100
	for _, w := range perCol {
		score -= w
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
