package quality

import (
	"fmt"
	"strings"
	"testing"
)

func csvColumn(name string, vals []float64) []byte {
	var b strings.Builder
	b.WriteString(name + "\n")
	for _, v := range vals {
		fmt.Fprintf(&b, "%g\n", v)
	}
	return []byte(b.String())
}

func authReport(c Check) map[string]any { return c.Report }

func findingsOf(c Check) []authFinding {
	if f, ok := c.Report["findings"].([]authFinding); ok {
		return f
	}
	return nil
}

func hasSignificant(c Check, detector string) bool {
	for _, f := range findingsOf(c) {
		if f.Detector == detector && f.Significant {
			return true
		}
	}
	return false
}

func TestAuthenticityCleanColumnPasses(t *testing.T) {
	// 0..199: last digits perfectly uniform, no sentinel, no dups.
	vals := make([]float64, 200)
	for i := range vals {
		vals[i] = float64(i)
	}
	c := Authenticity(csvColumn("value", vals), "text/csv")
	if c.Result != ResultPass {
		t.Errorf("clean column should pass, got %s (%v)", c.Result, authReport(c))
	}
	if band, _ := c.Report["band"].(string); band != "clean" {
		t.Errorf("band = %q, want clean", band)
	}
}

func TestAuthenticityFlagsFabricatedTerminalDigits(t *testing.T) {
	// Geng-style fabrication: every value ends in 0 or 5.
	vals := make([]float64, 200)
	for i := range vals {
		vals[i] = float64(i * 5)
	}
	c := Authenticity(csvColumn("yield", vals), "text/csv")
	if c.Result != ResultWarn {
		t.Errorf("fabricated terminal digits should warn, got %s (%v)", c.Result, authReport(c))
	}
	if band, _ := c.Report["band"].(string); band == "clean" {
		t.Errorf("band should not be clean, got %q", band)
	}
	if !hasSignificant(c, "terminal_digit_uniformity") {
		t.Errorf("expected a significant terminal_digit finding, got %+v", findingsOf(c))
	}
}

func TestAuthenticityFlagsSentinel(t *testing.T) {
	// Half the column is the -999 placeholder, mixed into varied real values.
	vals := make([]float64, 0, 200)
	for i := 0; i < 100; i++ {
		vals = append(vals, -999)
	}
	for i := 1; i <= 100; i++ {
		vals = append(vals, float64(i)+0.123) // varied, non-integer so terminal-digit skips
	}
	c := Authenticity(csvColumn("reading", vals), "text/csv")
	found := false
	for _, f := range findingsOf(c) {
		if f.Detector == "sentinel_value" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a sentinel_value finding, got %+v", findingsOf(c))
	}
	if c.Result != ResultWarn {
		t.Errorf("sentinel-laden column should warn, got %s", c.Result)
	}
}

func TestAuthenticityFlagsDuplicateRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("a,b\n")
	for i := 0; i < 60; i++ {
		b.WriteString("1,2\n") // every data row identical
	}
	c := Authenticity([]byte(b.String()), "text/csv")
	found := false
	for _, f := range findingsOf(c) {
		if f.Detector == "duplicate_rows" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a duplicate_rows finding, got %+v", findingsOf(c))
	}
}

func TestAuthenticityNonCSVIsNotApplicable(t *testing.T) {
	c := Authenticity([]byte(`{"a":1}`), "application/json")
	if c.Result != ResultPass {
		t.Errorf("non-CSV should pass, got %s", c.Result)
	}
	if applicable, _ := c.Report["applicable"].(bool); applicable {
		t.Errorf("non-CSV should be not-applicable, got %v", c.Report)
	}
}

func TestAuthenticityTooSmallIsNotApplicable(t *testing.T) {
	c := Authenticity(csvColumn("v", []float64{1, 2, 3, 4, 5}), "text/csv")
	if applicable, _ := c.Report["applicable"].(bool); applicable {
		t.Errorf("tiny dataset should be not-applicable, got %v", c.Report)
	}
}

func TestAuthenticityNeverFails(t *testing.T) {
	// Pathological all-zero column must not produce a fail (only pass/warn).
	vals := make([]float64, 50)
	c := Authenticity(csvColumn("z", vals), "text/csv")
	if c.Result == ResultFail {
		t.Errorf("authenticity must never fail/bounce a dataset, got %s", c.Result)
	}
}

func TestAuthenticityTSV(t *testing.T) {
	var b strings.Builder
	b.WriteString("idx\tyield\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "%d\t%d\n", i, i*5) // yield ends in 0/5 (fabricated)
	}
	c := Authenticity([]byte(b.String()), "text/tab-separated-values")
	if applicable, _ := c.Report["applicable"].(bool); !applicable {
		t.Fatalf("TSV should be screenable (delimiter used), got %v", c.Report)
	}
	if c.Result != ResultWarn || c.Report["band"] == "clean" {
		t.Errorf("fabricated TSV should warn/non-clean, got %s %v", c.Result, c.Report)
	}
	if !hasSignificant(c, "terminal_digit_uniformity") {
		t.Errorf("expected terminal_digit finding in TSV, got %+v", findingsOf(c))
	}
}

func TestAuthenticityJSONLNotTabular(t *testing.T) {
	c := Authenticity([]byte(`{"a":1}`+"\n"+`{"a":2}`), "application/x-ndjson")
	if applicable, _ := c.Report["applicable"].(bool); applicable {
		t.Errorf("JSONL is not tabular — screening should be not-applicable, got %v", c.Report)
	}
}

func TestAuthenticityParquetDefersToSidecar(t *testing.T) {
	// Parquet can't be read in-process; Go reports not-applicable with a note
	// that the sidecar handles it (the worker overlays the sidecar result).
	blob := append(append([]byte("PAR1"), []byte{0x00, 0x01}...), []byte("PAR1")...)
	c := Authenticity(blob, "application/vnd.apache.parquet")
	if applicable, _ := c.Report["applicable"].(bool); applicable {
		t.Errorf("parquet should be not-applicable in the Go baseline, got %v", c.Report)
	}
	if note, _ := c.Report["note"].(string); !strings.Contains(note, "sidecar") {
		t.Errorf("note should mention the sidecar, got %q", note)
	}
}

func TestTerminalDigit_SkipsLowCardinalityIntegers(t *testing.T) {
	// Binary 0/1, Likert 1-5, day-of-week 0-6, month 1-12 are categorical/coded
	// columns. The last-digit-uniformity test assumes a uniform terminal digit under
	// the null, which is FALSE for small-range/low-cardinality integers — so it must
	// NOT fire (the same false-positive class as the GPS PII bug, on the auth path).
	mk := func(mod, add int) numColumn {
		var c numColumn
		c.name = "coded"
		for i := 0; i < 300; i++ {
			c.values = append(c.values, float64(i%mod+add))
		}
		return c
	}
	for _, c := range []numColumn{mk(2, 0), mk(5, 1), mk(7, 0), mk(12, 1)} {
		if f, ok := terminalDigitFinding(c); ok {
			t.Fatalf("terminal-digit wrongly applicable to low-cardinality column (mod path): %+v", f)
		}
	}
}

func TestTerminalDigit_StillFiresOnWideRangeDigitStacking(t *testing.T) {
	// A wide-range, high-cardinality integer column whose values are all multiples of
	// 10 (terminal digit always 0 — fabrication-like) must STILL be detected.
	var c numColumn
	c.name = "amount"
	for i := 0; i < 300; i++ {
		c.values = append(c.values, float64((i*70)%9000)) // 0..8990 step 70, all end in 0
	}
	if _, ok := terminalDigitFinding(c); !ok {
		t.Fatal("terminal-digit should still apply+fire on wide-range digit-stacked integers")
	}
}

func TestDuplicateRows_ExcludesHeaderFromRatio(t *testing.T) {
	// 21 distinct data rows + 9 rows duplicating an existing one => 30 data rows,
	// true duplicate ratio = 9/30 = 0.30 (== threshold, should fire). Counting the
	// header row would dilute it to 9/31 = 0.29 and wrongly SUPPRESS the finding.
	var b strings.Builder
	b.WriteString("a,b,c\n") // header (non-numeric => detected)
	for i := 0; i < 21; i++ {
		fmt.Fprintf(&b, "%d,%d,%d\n", i, i, i) // 21 distinct
	}
	for i := 0; i < 9; i++ {
		b.WriteString("0,0,0\n") // 9 duplicates of the i==0 row
	}
	f, ok := duplicateRowsFinding([]byte(b.String()), 30, ',')
	if !ok {
		t.Fatal("duplicate-rows should fire at ratio 0.30 (header must be excluded)")
	}
	if got := f.Statistic; got < 0.30 {
		t.Fatalf("duplicate ratio computed on data rows should be 0.30, got %v (header counted?)", got)
	}
}
