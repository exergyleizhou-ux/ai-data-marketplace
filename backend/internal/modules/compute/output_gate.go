package compute

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// --- Anti-malicious-algorithm output gate (design A2) ---
//
// The sandbox severs the network (`--network=none`), so the output object is the
// ONLY exfiltration channel. The size cap alone defends a semi-honest buyer but
// not a MALICIOUS ALGORITHM AUTHOR, who can steganographically pack raw rows into
// a claimed "aggregate" (base64 a CSV into one field; emit a verbatim row-dump as
// a "model"). This gate bounds the *information content* of the output, fail-
// closed: a violation withholds the output and refunds the buyer (like the size
// gate). It NEVER mutates the output — detection + rejection only.
//
// It accepts exactly two shapes — the ones the live runners actually emit:
//   1. a single well-formed JSON object (MockRunner / federated / PSI sub-jobs);
//   2. a ZIP whose every entry is *.json and parses as JSON (the real algorithm
//      contract: output.bin = zip{model.json, metrics.json}).
// Everything else (raw blobs, CSVs, tarballs, a zip smuggling a .csv) is rejected.

// Output-gate reason codes. Reused as job reject codes (audited). ReasonTooLarge
// keeps the pre-existing "output_exceeds_max_bytes" string so the size-cap
// behavior and its tests/audit history are unchanged.
const (
	ReasonNotStructured     = "output_not_structured"
	ReasonTooLarge          = "output_exceeds_max_bytes"
	ReasonStringsTooLarge   = "output_strings_too_large"
	ReasonTooManyNumbers    = "output_too_many_numbers"
	ReasonTooManyKeys       = "output_too_many_keys"
	ReasonTooDeep           = "output_too_deep"
	ReasonHighEntropyString = "output_high_entropy_string"
)

// GatePolicy bounds an output's size, structure, and information content. Zero
// fields disable the corresponding check (the offer can only TIGHTEN MaxBytes via
// policyForKind; the rest are code-constant kind defaults — see the design doc).
type GatePolicy struct {
	MaxBytes            int64   // outer size cap on the raw (and decompressed) output
	MaxStringBytes      int     // total bytes of all string leaves (+ object keys)
	MaxNumericValues    int     // total numeric leaves
	MaxKeys             int     // total object keys
	MaxDepth            int     // max JSON nesting depth
	EntropyStringMinLen int     // only string leaves >= this length get the entropy check
	EntropyMaxBits      float64 // reject a checked string above this Shannon bits/byte
}

// GateViolation is a fail-closed gate rejection: a stable Reason code (becomes the
// job reject code) plus a human Detail (logged/audited, never billed).
type GateViolation struct {
	Reason string
	Detail string
}

func (v *GateViolation) Error() string { return v.Reason + ": " + v.Detail }

// policyForKind returns the default bounds for an output kind. A positive
// maxBytesOverride (the offer's MaxOutputBytes) tightens only the size cap.
func policyForKind(kind string, maxBytesOverride int64) GatePolicy {
	p := GatePolicy{
		MaxKeys:             5_000,
		MaxDepth:            12,
		EntropyStringMinLen: 256,
		EntropyMaxBits:      4.7,
	}
	switch kind {
	case OutputModel:
		// A real model is many numeric weights; allow more headroom.
		p.MaxBytes = 8 << 20
		p.MaxStringBytes = 64 << 10
		p.MaxNumericValues = 200_000
		p.MaxKeys = 50_000
		p.MaxDepth = 16
	default: // aggregate / metrics / table / unknown — an aggregate is O(k) stats
		p.MaxBytes = 1 << 20
		p.MaxStringBytes = 8 << 10
		p.MaxNumericValues = 10_000
	}
	if maxBytesOverride > 0 {
		p.MaxBytes = maxBytesOverride
	}
	return p
}

// GateOutput validates raw runner output against the policy for its kind. Returns
// a non-nil *GateViolation when the output must be withheld; nil when it passes.
func GateOutput(kind string, output []byte, p GatePolicy) *GateViolation {
	// 1. Outer size cap — cheapest, and an OOM bound. Checked first so an oversize
	//    non-JSON blob still reports the size reason (unchanged behavior).
	if p.MaxBytes > 0 && int64(len(output)) > p.MaxBytes {
		return &GateViolation{ReasonTooLarge, fmt.Sprintf("output is %d bytes, exceeds cap %d", len(output), p.MaxBytes)}
	}
	// 2. Structural shape: a single JSON object, or a zip of *.json.
	values, viol := decodeStructured(output, p)
	if viol != nil {
		return viol
	}
	// 3. Information-content bounds across all parsed JSON (the anti-exfil teeth).
	c := &counters{}
	for _, v := range values {
		if viol := walk(v, 0, p, c); viol != nil {
			return viol
		}
	}
	return nil
}

// decodeStructured enforces the container shape and returns the JSON value(s) to
// walk (the single object, or every zip entry's value).
func decodeStructured(output []byte, p GatePolicy) ([]any, *GateViolation) {
	if bytes.HasPrefix(output, []byte("PK\x03\x04")) { // ZIP local-file-header magic
		return decodeZip(output, p)
	}
	// A single JSON object. Unmarshalling into a map rejects arrays/scalars.
	dec := json.NewDecoder(bytes.NewReader(output))
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, &GateViolation{ReasonNotStructured, "output is neither a JSON object nor a zip-of-json: " + err.Error()}
	}
	if dec.More() {
		return nil, &GateViolation{ReasonNotStructured, "trailing content after the JSON object"}
	}
	return []any{obj}, nil
}

// decodeZip validates that every entry is a *.json file of well-formed JSON,
// bounding entry count and decompressed size (zip-bomb defense).
func decodeZip(output []byte, p GatePolicy) ([]any, *GateViolation) {
	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	if err != nil {
		return nil, &GateViolation{ReasonNotStructured, "invalid zip: " + err.Error()}
	}
	if n := len(zr.File); n == 0 || n > 64 {
		return nil, &GateViolation{ReasonNotStructured, fmt.Sprintf("zip has %d entries (want 1..64)", n)}
	}
	limit := p.MaxBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	var values []any
	var total int64
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".json") {
			return nil, &GateViolation{ReasonNotStructured, "zip entry is not .json: " + f.Name}
		}
		rc, err := f.Open()
		if err != nil {
			return nil, &GateViolation{ReasonNotStructured, "zip open " + f.Name + ": " + err.Error()}
		}
		data, err := io.ReadAll(io.LimitReader(rc, limit+1))
		rc.Close()
		if err != nil {
			return nil, &GateViolation{ReasonNotStructured, "zip read " + f.Name + ": " + err.Error()}
		}
		total += int64(len(data))
		if int64(len(data)) > limit || total > limit {
			return nil, &GateViolation{ReasonTooLarge, "decompressed zip exceeds cap (possible zip bomb)"}
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, &GateViolation{ReasonNotStructured, "zip entry is not valid json: " + f.Name}
		}
		values = append(values, v)
	}
	return values, nil
}

// counters accumulates information-content tallies across the whole output.
type counters struct {
	stringBytes int
	numbers     int
	keys        int
}

// walk recurses the parsed JSON, enforcing the magnitude + entropy bounds and
// short-circuiting on the first violation (so a deep/huge tree can't exhaust the
// stack or CPU — depth is checked before recursing).
func walk(v any, depth int, p GatePolicy, c *counters) *GateViolation {
	if p.MaxDepth > 0 && depth > p.MaxDepth {
		return &GateViolation{ReasonTooDeep, fmt.Sprintf("nesting exceeds %d", p.MaxDepth)}
	}
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			c.keys++
			if p.MaxKeys > 0 && c.keys > p.MaxKeys {
				return &GateViolation{ReasonTooManyKeys, fmt.Sprintf("more than %d keys", p.MaxKeys)}
			}
			c.stringBytes += len(k) // a key can carry exfil too
			if p.MaxStringBytes > 0 && c.stringBytes > p.MaxStringBytes {
				return &GateViolation{ReasonStringsTooLarge, fmt.Sprintf("string content exceeds %d bytes", p.MaxStringBytes)}
			}
			if viol := walk(val, depth+1, p, c); viol != nil {
				return viol
			}
		}
	case []any:
		for _, val := range x {
			if viol := walk(val, depth+1, p, c); viol != nil {
				return viol
			}
		}
	case string:
		c.stringBytes += len(x)
		if p.MaxStringBytes > 0 && c.stringBytes > p.MaxStringBytes {
			return &GateViolation{ReasonStringsTooLarge, fmt.Sprintf("string content exceeds %d bytes", p.MaxStringBytes)}
		}
		if p.EntropyStringMinLen > 0 && len(x) >= p.EntropyStringMinLen && shannonBits(x) > p.EntropyMaxBits {
			return &GateViolation{ReasonHighEntropyString, fmt.Sprintf("a %d-byte string looks like an encoded/compressed blob", len(x))}
		}
	case float64, json.Number:
		c.numbers++
		if p.MaxNumericValues > 0 && c.numbers > p.MaxNumericValues {
			return &GateViolation{ReasonTooManyNumbers, fmt.Sprintf("more than %d numeric values", p.MaxNumericValues)}
		}
	case bool, nil:
		// no information-bearing payload
	}
	return nil
}

// shannonBits is the order-0 Shannon entropy of s in bits per byte. A uniform
// base64/compressed payload approaches 6–8; natural text and repetitive labels
// stay well below the gate threshold.
func shannonBits(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	var freq [256]int
	for i := 0; i < len(s); i++ {
		freq[s[i]]++
	}
	n := float64(len(s))
	h := 0.0
	for _, f := range freq {
		if f == 0 {
			continue
		}
		pp := float64(f) / n
		h -= pp * math.Log2(pp)
	}
	return h
}
