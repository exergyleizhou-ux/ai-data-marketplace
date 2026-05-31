// Package audit provides the append-only audit trail required for compliance
// (docs §6.8: uploads, reviews, trades, settlements, downloads, dispute
// rulings all get logged). The audit_logs table is append-only at the DB level
// (a trigger blocks UPDATE/DELETE).
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one audit record. ActorID/IP may be empty (rendered as SQL NULL).
type Entry struct {
	ActorID      string
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   string
	IP           string
	UserAgent    string
	Detail       any
}

// Recorder appends audit entries. Implementations must be safe for concurrent
// use and must never panic on a bad write — auditing is best-effort at the call
// site but failures are logged loudly for follow-up.
type Recorder interface {
	Record(ctx context.Context, e Entry)
}

type pgRecorder struct{ pool *pgxpool.Pool }

// New returns a Postgres-backed Recorder.
func New(pool *pgxpool.Pool) Recorder { return &pgRecorder{pool: pool} }

func (r *pgRecorder) Record(ctx context.Context, e Entry) {
	detail, _ := json.Marshal(e.Detail)
	const q = `
		INSERT INTO audit_logs (actor_id, actor_role, action, resource_type, resource_id, ip, user_agent, detail)
		VALUES (NULLIF($1,'')::uuid, NULLIF($2,''), $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,'')::inet, NULLIF($7,''), $8)`
	if _, err := r.pool.Exec(ctx, q,
		e.ActorID, e.ActorRole, e.Action, e.ResourceType, e.ResourceID, e.IP, e.UserAgent, detail,
	); err != nil {
		// Compliance trail: do not fail the user action, but make the loss visible.
		slog.Error("audit log write failed", "action", e.Action, "resource", e.ResourceType+":"+e.ResourceID, "err", err)
	}
}

// Noop discards entries — for tests and route-only servers.
type Noop struct{}

func (Noop) Record(context.Context, Entry) {}
