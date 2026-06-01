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
