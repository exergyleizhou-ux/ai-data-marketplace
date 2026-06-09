package anomaly

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Anomaly is a detected audit pattern.
type Anomaly struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	ActorID         string  `json:"actor_id,omitempty"`
	ResourcePattern string  `json:"resource_pattern"`
	SampleAuditIDs  []int64 `json:"sample_audit_ids"`
	Count           int     `json:"count"`
	FirstSeenAt     string  `json:"first_seen_at"`
	LastSeenAt      string  `json:"last_seen_at"`
	Status          string  `json:"status"`
	OpsNote         string  `json:"ops_note,omitempty"`
	CreatedAt       string  `json:"created_at,omitempty"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
}

// Rule detects anomalies in a recent time window.
type Rule interface {
	Kind() string
	Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error)
}

// DBQuerier is the query interface the rules read from.
type DBQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
