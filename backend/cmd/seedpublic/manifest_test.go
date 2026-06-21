package main

import (
	"encoding/csv"
	"strings"
	"testing"
)

func TestNormalizeToCSV_PassthroughLeavesCommaDataUnchanged(t *testing.T) {
	in := []byte("5.1,3.5,1.4,0.2,Iris-setosa\n4.9,3.0,1.4,0.2,Iris-setosa\n")
	out, err := normalizeToCSV(in, formatCSV)
	if err != nil {
		t.Fatalf("normalizeToCSV: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("passthrough changed data:\n got: %q\nwant: %q", out, in)
	}
}

func TestNormalizeToCSV_SemicolonBecomesComma(t *testing.T) {
	in := []byte(`"fixed acidity";"volatile acidity";"quality"` + "\n7.4;0.7;5\n")
	out, err := normalizeToCSV(in, formatSemicolon)
	if err != nil {
		t.Fatalf("normalizeToCSV: %v", err)
	}
	want := "fixed acidity,volatile acidity,quality\n7.4,0.7,5\n"
	if string(out) != want {
		t.Fatalf("semicolon normalize:\n got: %q\nwant: %q", out, want)
	}
}

func TestNormalizeToCSV_WhitespaceRunsBecomeComma(t *testing.T) {
	// seeds_dataset.txt uses tabs; ecoli uses runs of spaces. A leading name
	// token with no internal whitespace must be preserved as a single field.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tabs", "15.26\t14.84\t0.871\n", "15.26,14.84,0.871\n"},
		{"double-space", "0.49  0.29  0.48\n", "0.49,0.29,0.48\n"},
		{"name-prefix", "AAT_ECOLI   0.49  0.29\n", "AAT_ECOLI,0.49,0.29\n"},
		{"trims-edges", "  1.0\t2.0  \n", "1.0,2.0\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := normalizeToCSV([]byte(c.in), formatWhitespace)
			if err != nil {
				t.Fatalf("normalizeToCSV: %v", err)
			}
			if string(out) != c.want {
				t.Fatalf("whitespace normalize:\n got: %q\nwant: %q", out, c.want)
			}
		})
	}
}

func TestNormalizeToCSV_OutputIsParseableCSV(t *testing.T) {
	in := []byte("AAT_ECOLI   0.49  0.29  0.48\nACEA_ECOLI  0.07  0.40  0.48\n")
	out, err := normalizeToCSV(in, formatWhitespace)
	if err != nil {
		t.Fatalf("normalizeToCSV: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		t.Fatalf("normalized output not valid CSV: %v", err)
	}
	if len(rows) != 2 || len(rows[0]) != 4 {
		t.Fatalf("unexpected shape: %d rows, first row %d cols", len(rows), len(rows[0]))
	}
}

func TestNormalizeToCSV_RejectsUnknownFormat(t *testing.T) {
	if _, err := normalizeToCSV([]byte("x"), sourceFormat("xls")); err == nil {
		t.Fatal("expected error for unknown source format, got nil")
	}
}

func TestPrependHeader(t *testing.T) {
	if got := prependHeader([]byte("1,2\n3,4\n"), "a,b"); string(got) != "a,b\n1,2\n3,4\n" {
		t.Fatalf("prependHeader: got %q", got)
	}
	// An empty header (dataset already has one) leaves the bytes untouched.
	if got := prependHeader([]byte("x,y\n1,2\n"), ""); string(got) != "x,y\n1,2\n" {
		t.Fatalf("prependHeader empty: got %q", got)
	}
}

func TestSeedManifest_EntriesValid(t *testing.T) {
	if len(seedDatasets) < 8 {
		t.Fatalf("want at least 8 seed datasets across research verticals, got %d", len(seedDatasets))
	}
	validLicense := map[string]bool{"commercial": true, "research": true, "train_only": true}
	keys := map[string]bool{}
	domains := map[string]bool{}
	for _, d := range seedDatasets {
		if d.Key == "" || d.TitleZH == "" || d.TitleEN == "" || d.Domain == "" ||
			d.SourceURL == "" || d.LicenseType == "" || d.LicenseNote == "" || d.Citation == "" ||
			d.DescZH == "" || d.DescEN == "" {
			t.Fatalf("seed %q has an empty required field: %+v", d.Key, d)
		}
		if d.DataType != "structured" {
			t.Fatalf("seed %q: data_type=%q, want structured (CSV is tabular)", d.Key, d.DataType)
		}
		// license_type must satisfy the datasets CHECK constraint
		// (commercial|research|train_only); human-readable terms go in LicenseNote.
		if !validLicense[d.LicenseType] {
			t.Fatalf("seed %q: license_type=%q violates the DB CHECK (commercial|research|train_only)", d.Key, d.LicenseType)
		}
		switch d.Format {
		case formatCSV, formatSemicolon, formatWhitespace:
		default:
			t.Fatalf("seed %q: invalid format %q", d.Key, d.Format)
		}
		if d.PriceCents <= 0 {
			t.Fatalf("seed %q: price_cents must be positive, got %d", d.Key, d.PriceCents)
		}
		if !strings.HasPrefix(d.SourceURL, "https://") {
			t.Fatalf("seed %q: source must be https, got %q", d.Key, d.SourceURL)
		}
		if keys[d.Key] {
			t.Fatalf("duplicate seed key %q", d.Key)
		}
		keys[d.Key] = true
		domains[d.Domain] = true
	}
	if len(domains) < 8 {
		t.Fatalf("want at least 8 distinct research verticals, got %d", len(domains))
	}
}
