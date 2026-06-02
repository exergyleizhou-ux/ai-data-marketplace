package quality

import (
	"bytes"
	"encoding/csv"
	"math"
	"strconv"
	"strings"
)

// Schema profiling bounds — keep time/memory predictable on large files.
const (
	schemaMaxRows     = 200000 // rows scanned for the profile
	schemaDistinctCap = 2048   // stop counting distinct values past this
	schemaSamples     = 3      // example values kept per non-numeric column
)

// columnProfile is one column's inferred shape, shown to buyers pre-purchase.
type columnProfile struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"` // integer | number | boolean | string | empty
	NonNull        int      `json:"non_null"`
	Null           int      `json:"null"`
	Distinct       int      `json:"distinct"`
	DistinctCapped bool     `json:"distinct_capped,omitempty"`
	Min            *float64 `json:"min,omitempty"`
	Max            *float64 `json:"max,omitempty"`
	Mean           *float64 `json:"mean,omitempty"`
	MaxLen         int      `json:"max_len,omitempty"`
	Samples        []string `json:"samples,omitempty"`
}

// Schema profiles the columns of a tabular dataset (CSV/TSV): name, inferred
// type, null counts, a (capped) distinct estimate, and numeric/string stats.
// Result is always pass — this is descriptive profiling, never a gate.
// Non-tabular payloads report applicable:false.
func Schema(content []byte, contentType string) Check {
	delim, ok := tabularDelimiter(contentType)
	if !ok {
		return Check{Type: TypeSchema, Result: ResultPass, Report: map[string]any{"applicable": false}}
	}
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = delim
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return Check{Type: TypeSchema, Result: ResultPass, Report: map[string]any{"applicable": false}}
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

	cols := make([]columnProfile, 0, ncol)
	for j := 0; j < ncol; j++ {
		cols = append(cols, profileColumn(rows, start, j, header))
	}
	rowCount := len(rows) - start
	if rowCount > schemaMaxRows {
		rowCount = schemaMaxRows
	}
	return Check{Type: TypeSchema, Result: ResultPass, Report: map[string]any{
		"applicable":   true,
		"row_count":    rowCount,
		"column_count": ncol,
		"columns":      cols,
	}}
}

func profileColumn(rows [][]string, start, j int, header bool) columnProfile {
	p := columnProfile{Name: columnName(rows, j, header)}
	allInt, allNum, allBool := true, true, true
	distinct := map[string]struct{}{}
	capped := false
	sampleSet := map[string]struct{}{}
	var sum, mn, mx float64
	var nNum int

	processed := 0
	for i := start; i < len(rows); i++ {
		if processed >= schemaMaxRows {
			break
		}
		processed++
		cell := ""
		if j < len(rows[i]) {
			cell = strings.TrimSpace(rows[i][j])
		}
		if cell == "" {
			p.Null++
			continue
		}
		p.NonNull++

		if !capped {
			if _, seen := distinct[cell]; !seen {
				if len(distinct) >= schemaDistinctCap {
					capped = true
				} else {
					distinct[cell] = struct{}{}
				}
			}
		}
		if allInt {
			if _, err := strconv.ParseInt(cell, 10, 64); err != nil {
				allInt = false
			}
		}
		if allNum {
			if f, err := strconv.ParseFloat(cell, 64); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				allNum = false
			} else {
				nNum++
				sum += f
				if nNum == 1 || f < mn {
					mn = f
				}
				if nNum == 1 || f > mx {
					mx = f
				}
			}
		}
		if allBool && !isBoolLiteral(cell) {
			allBool = false
		}
		if l := len([]rune(cell)); l > p.MaxLen {
			p.MaxLen = l
		}
		if len(p.Samples) < schemaSamples {
			if _, seen := sampleSet[cell]; !seen {
				sampleSet[cell] = struct{}{}
				p.Samples = append(p.Samples, cell)
			}
		}
	}

	switch {
	case p.NonNull == 0:
		p.Type = "empty"
	case allInt:
		p.Type = "integer"
	case allNum:
		p.Type = "number"
	case allBool:
		p.Type = "boolean"
	default:
		p.Type = "string"
	}
	p.Distinct = len(distinct)
	p.DistinctCapped = capped

	if (p.Type == "integer" || p.Type == "number") && nNum > 0 {
		lo, hi, mean := round4(mn), round4(mx), round4(sum/float64(nNum))
		p.Min, p.Max, p.Mean = &lo, &hi, &mean
		p.Samples = nil // numeric columns show stats, not samples
		p.MaxLen = 0
	}
	return p
}

func columnName(rows [][]string, j int, header bool) string {
	if header && j < len(rows[0]) {
		if name := strings.TrimSpace(rows[0][j]); name != "" {
			return name
		}
	}
	return "col_" + strconv.Itoa(j)
}

func isBoolLiteral(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no":
		return true
	default:
		return false
	}
}

func round4(f float64) float64 {
	return math.Round(f*10000) / 10000
}
