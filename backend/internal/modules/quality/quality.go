// Package quality is a stateless library of data-quality checks: format/
// encoding validation, basic statistics, PII scanning, and content
// fingerprinting (SHA-256 is done by storage; SimHash here for near-dup).
//
// It has no DB or storage dependencies — the dataset module reads the object,
// calls these functions, persists quality_check rows, and advances the dataset
// state (docs §6.3). Production moves orchestration to an Asynq worker; the
// check logic stays here unchanged.
package quality

const (
	ResultPass = "pass"
	ResultWarn = "warn"
	ResultFail = "fail"

	TypeFormat = "format"
	TypeStats  = "stats"
	TypeDedup  = "dedup"
	TypePII    = "pii"
)

// Check is one check's outcome, ready to persist as a quality_check row.
type Check struct {
	Type   string         `json:"type"`
	Result string         `json:"result"`
	Report map[string]any `json:"report"`
}
