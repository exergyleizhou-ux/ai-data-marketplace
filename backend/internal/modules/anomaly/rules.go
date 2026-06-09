package anomaly

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// RepeatedFailureRule detects actors with >=10 failure-type actions in the window.
type RepeatedFailureRule struct{}

func (r *RepeatedFailureRule) Kind() string { return "repeated_failure" }

func (r *RepeatedFailureRule) Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error) {
	rows, err := db.Query(ctx,
		`SELECT COALESCE(actor_id::text,''), action, COUNT(*) as cnt,
			MIN(created_at) as first_at, MAX(created_at) as last_at,
			ARRAY_AGG(id ORDER BY created_at DESC LIMIT 5) as sample_ids
		 FROM audit_logs
		 WHERE created_at >= $1
		   AND (action LIKE '%reject' OR action LIKE '%fail' OR action LIKE '%error')
		   AND actor_id IS NOT NULL
		 GROUP BY actor_id, action
		 HAVING COUNT(*) >= 10`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnomalies(rows, "repeated_failure")
}

// BulkModificationRule detects >=20 distinct resources modified by same actor+action.
type BulkModificationRule struct{}

func (r *BulkModificationRule) Kind() string { return "bulk_modification" }

func (r *BulkModificationRule) Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error) {
	rows, err := db.Query(ctx,
		`SELECT COALESCE(actor_id::text,''), action || '.' || COALESCE(resource_type,'?'),
			COUNT(DISTINCT resource_id) as cnt,
			MIN(created_at) as first_at, MAX(created_at) as last_at,
			ARRAY_AGG(id ORDER BY created_at DESC LIMIT 5) as sample_ids
		 FROM audit_logs
		 WHERE created_at >= $1
		   AND actor_id IS NOT NULL AND resource_id IS NOT NULL
		 GROUP BY actor_id, action, resource_type
		 HAVING COUNT(DISTINCT resource_id) >= 20`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnomalies(rows, "bulk_modification")
}

// HighRiskActionRule logs every occurrence of a high-risk action.
type HighRiskActionRule struct{}

func (r *HighRiskActionRule) Kind() string { return "high_risk_action" }

func (r *HighRiskActionRule) Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error) {
	rows, err := db.Query(ctx,
		`SELECT COALESCE(actor_id::text,''), action, COALESCE(resource_type,'?') || ':' || COALESCE(resource_id,'-'),
			1 as cnt, created_at as first_at, created_at as last_at,
			ARRAY[id] as sample_ids
		 FROM audit_logs
		 WHERE created_at >= $1
		   AND action IN ('dataset.reject', 'kyc.reject', 'withdrawal.reject', 'dataset.delist')`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnomalies(rows, "high_risk_action")
}

func scanAnomalies(rows pgx.Rows, kind string) ([]Anomaly, error) {
	var out []Anomaly
	for rows.Next() {
		var a Anomaly
		a.Kind = kind
		var firstAt, lastAt time.Time
		if err := rows.Scan(&a.ActorID, &a.ResourcePattern, &a.Count,
			&firstAt, &lastAt, &a.SampleAuditIDs); err != nil {
			return nil, err
		}
		a.FirstSeenAt = firstAt.UTC().Format(time.RFC3339)
		a.LastSeenAt = lastAt.UTC().Format(time.RFC3339)
		out = append(out, a)
	}
	return out, rows.Err()
}
