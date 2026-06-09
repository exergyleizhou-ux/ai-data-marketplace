package auditlog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides read-only access to audit_logs.
type Repository interface {
	List(ctx context.Context, f ListFilter) ([]LogEntry, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) List(ctx context.Context, f ListFilter) ([]LogEntry, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	const q = `
		SELECT id, COALESCE(actor_id::text, ''), COALESCE(actor_role, ''), action,
			COALESCE(resource_type, ''), COALESCE(resource_id, ''),
			COALESCE(host(ip), ''), COALESCE(user_agent, ''),
			COALESCE(detail, '{}'::jsonb),
			created_at::text
		FROM audit_logs
		WHERE ($1 = '' OR actor_id::text = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR resource_type = $3)
		  AND ($4 = '' OR resource_id = $4)
		  AND ($5 = '' OR created_at >= $5::timestamptz)
		  AND ($6 = '' OR created_at <  $6::timestamptz)
		ORDER BY created_at DESC, id DESC
		LIMIT $7 OFFSET $8`
	rows, err := r.pool.Query(ctx, q,
		f.ActorID, f.Action, f.ResourceType, f.ResourceID,
		f.From, f.To,
		f.Limit, f.Offset)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		var rawDetail []byte
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorRole, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.IP, &e.UserAgent,
			&rawDetail, &e.CreatedAt); err != nil {
			return nil, err
		}
		if len(rawDetail) > 0 {
			var m map[string]any
			if err := json.Unmarshal(rawDetail, &m); err == nil {
				e.Detail = m
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
