package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/modules/quality"
)

// authenticityScreener produces an authenticity Check for tabular content. The
// HTTP implementation calls the PaperGuard sidecar; when it is unavailable the
// worker falls back to the in-process Go baseline (quality.Authenticity), so the
// authenticity score is always populated and the sidecar is never on the
// critical path. Both engines share the 0-100 score + clean/review/suspect band
// so results are comparable regardless of which one ran.
type authenticityScreener interface {
	Screen(ctx context.Context, content []byte, contentType string) (quality.Check, error)
}

// httpScreener calls the PaperGuard sidecar's POST /v1/screen endpoint.
type httpScreener struct {
	url    string
	client *http.Client
}

func newHTTPScreener(url string, timeout time.Duration) *httpScreener {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &httpScreener{url: url, client: &http.Client{Timeout: timeout}}
}

// sidecarResponse mirrors the sidecar contract (services/paperguard-sidecar).
type sidecarResponse struct {
	SchemaVersion string `json:"schema_version"`
	Engine        any    `json:"engine"`
	Summary       struct {
		AuthenticityScore int    `json:"authenticity_score"`
		Band              string `json:"band"`
		NFindings         int    `json:"n_findings"`
		ColumnsScreened   int    `json:"columns_screened"`
		Rows              int    `json:"rows"`
		Truncated         bool   `json:"truncated"`
	} `json:"summary"`
	Findings []any `json:"findings"`
	Errors   []any `json:"errors"`
}

// Screen posts the content to the sidecar and maps the response to a quality
// Check. It returns an error on any transport/decoding/contract problem so the
// caller can fall back; it never panics on a partial response.
func (h *httpScreener) Screen(ctx context.Context, content []byte, contentType string) (quality.Check, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url+"/v1/screen", bytes.NewReader(content))
	if err != nil {
		return quality.Check{}, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return quality.Check{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // cap response at 8 MiB
	if err != nil {
		return quality.Check{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return quality.Check{}, fmt.Errorf("sidecar status %d: %s", resp.StatusCode, truncate(body, 256))
	}

	var sr sidecarResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return quality.Check{}, fmt.Errorf("decode sidecar response: %w", err)
	}
	if sr.Summary.Band == "" {
		return quality.Check{}, fmt.Errorf("sidecar response missing band")
	}

	// clean -> pass; review/suspect -> warn. Like the Go baseline, the sidecar
	// is advisory and must never fail/bounce a dataset.
	result := quality.ResultPass
	if sr.Summary.Band != "clean" {
		result = quality.ResultWarn
	}
	report := map[string]any{
		"engine":           "paperguard-sidecar",
		"paperguard":       sr.Engine,
		"score":            sr.Summary.AuthenticityScore,
		"band":             sr.Summary.Band,
		"n_findings":       sr.Summary.NFindings,
		"columns_screened": sr.Summary.ColumnsScreened,
		"rows":             sr.Summary.Rows,
		"truncated":        sr.Summary.Truncated,
		"findings":         sr.Findings,
		"errors":           sr.Errors,
		"applicable":       true,
	}
	return quality.Check{Type: quality.TypeAuthenticity, Result: result, Report: report}, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
