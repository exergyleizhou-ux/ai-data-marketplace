package anomaly

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Upsert(ctx context.Context, a Anomaly) error
	List(ctx context.Context, status string, limit, offset int) ([]Anomaly, error)
	Get(ctx context.Context, id string) (Anomaly, error)
	SetStatus(ctx context.Context, id, status, opsID, note string) (Anomaly, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) Upsert(ctx context.Context, a Anomaly) error {
	firstAt, _ := time.Parse(time.RFC3339, a.FirstSeenAt)
	lastAt, _ := time.Parse(time.RFC3339, a.LastSeenAt)
	if firstAt.IsZero() {
		return fmt.Errorf("invalid first_seen_at")
	}
	if lastAt.IsZero() {
		lastAt = firstAt
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_anomalies (kind, actor_id, resource_pattern, sample_audit_ids, count,
			first_seen_at, last_seen_at, status)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4::bigint[], $5, $6, $7, 'open')
		 ON CONFLICT (kind, COALESCE(actor_id::text,''), resource_pattern) WHERE status = 'open' DO UPDATE
		 SET count = EXCLUDED.count,
		     last_seen_at = GREATEST(audit_anomalies.last_seen_at, EXCLUDED.last_seen_at),
		     sample_audit_ids = EXCLUDED.sample_audit_ids,
		     updated_at = now()`,
		a.Kind, a.ActorID, a.ResourcePattern, a.SampleAuditIDs, a.Count,
		firstAt, lastAt)
	if err != nil {
		return fmt.Errorf("upsert anomaly: %w", err)
	}
	return nil
}

func (r *pgRepo) List(ctx context.Context, status string, limit, offset int) ([]Anomaly, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var (
		rows pgx.Rows
		err  error
	)
	if status == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id::text, kind, COALESCE(actor_id::text,''), resource_pattern,
			    sample_audit_ids, count, first_seen_at::text, last_seen_at::text,
			    status, COALESCE(ops_note,''), created_at::text, updated_at::text
			 FROM audit_anomalies ORDER BY last_seen_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id::text, kind, COALESCE(actor_id::text,''), resource_pattern,
			    sample_audit_ids, count, first_seen_at::text, last_seen_at::text,
			    status, COALESCE(ops_note,''), created_at::text, updated_at::text
			 FROM audit_anomalies WHERE status=$1 ORDER BY last_seen_at DESC LIMIT $2 OFFSET $3`,
			status, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list anomalies: %w", err)
	}
	defer rows.Close()
	var out []Anomaly
	for rows.Next() {
		var a Anomaly
		if err := rows.Scan(&a.ID, &a.Kind, &a.ActorID, &a.ResourcePattern,
			&a.SampleAuditIDs, &a.Count, &a.FirstSeenAt, &a.LastSeenAt,
			&a.Status, &a.OpsNote, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *pgRepo) Get(ctx context.Context, id string) (Anomaly, error) {
	var a Anomaly
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, kind, COALESCE(actor_id::text,''), resource_pattern,
		    sample_audit_ids, count, first_seen_at::text, last_seen_at::text,
		    status, COALESCE(ops_note,''), created_at::text, updated_at::text
		 FROM audit_anomalies WHERE id=$1`, id).
		Scan(&a.ID, &a.Kind, &a.ActorID, &a.ResourcePattern,
			&a.SampleAuditIDs, &a.Count, &a.FirstSeenAt, &a.LastSeenAt,
			&a.Status, &a.OpsNote, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Anomaly{}, fmt.Errorf("get anomaly: %w", err)
	}
	return a, nil
}

func (r *pgRepo) SetStatus(ctx context.Context, id, status, opsID, note string) (Anomaly, error) {
	var a Anomaly
	err := r.pool.QueryRow(ctx,
		`UPDATE audit_anomalies SET status=$2, ops_note=$3, updated_at=now()
		 WHERE id=$1 RETURNING id::text, kind, COALESCE(actor_id::text,''), resource_pattern,
		 sample_audit_ids, count, first_seen_at::text, last_seen_at::text,
		 status, COALESCE(ops_note,''), created_at::text, updated_at::text`,
		id, status, note).
		Scan(&a.ID, &a.Kind, &a.ActorID, &a.ResourcePattern,
			&a.SampleAuditIDs, &a.Count, &a.FirstSeenAt, &a.LastSeenAt,
			&a.Status, &a.OpsNote, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Anomaly{}, fmt.Errorf("set anomaly status: %w", err)
	}
	return a, nil
}
