package anomaly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Alerter sends anomaly alerts to an external system (webhook).
type Alerter interface {
	Alert(ctx context.Context, a Anomaly) error
}

// WebhookAlerter POSTs new anomalies to a configured URL.
type WebhookAlerter struct {
	url    string
	kinds  map[string]bool
	client *http.Client
}

// NewWebhookAlerter creates an alerter that fires for kinds in the whitelist.
// Pass nil kinds to alert on all kinds.
func NewWebhookAlerter(url string, kinds []string) Alerter {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return &WebhookAlerter{
		url:    url,
		kinds:  m,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *WebhookAlerter) Alert(ctx context.Context, an Anomaly) error {
	if len(a.kinds) > 0 && !a.kinds[an.Kind] {
		return nil // kind not in whitelist, no alert
	}
	payload := map[string]any{
		"kind":             an.Kind,
		"actor_id":         an.ActorID,
		"resource_pattern": an.ResourcePattern,
		"count":            an.Count,
		"first_seen_at":    an.FirstSeenAt,
		"last_seen_at":     an.LastSeenAt,
		"sample_audit_ids": an.SampleAuditIDs,
		"anomaly_id":       an.ID,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", a.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// NopAlerter discards all alerts.
type NopAlerter struct{}

func (NopAlerter) Alert(_ context.Context, _ Anomaly) error { return nil }

// alertNew sends alerts only for newly created (not updated) anomalies.
func alertNew(alerter Alerter, ctx context.Context, a Anomaly) {
	if alerter == nil {
		return
	}
	if err := alerter.Alert(ctx, a); err != nil {
		slog.Warn("anomaly alert failed", "kind", a.Kind, "anomaly_id", a.ID, "err", err)
	}
}
