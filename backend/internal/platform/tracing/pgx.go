package tracing

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// maxStmtLen caps the recorded db.statement attribute. Queries here are
// parameterized ($N placeholders, no literal values), so this is belt-and-
// braces against accidentally recording huge SQL, not a PII control.
const maxStmtLen = 120

// PgxTracer implements pgx.QueryTracer, emitting one child span per query.
// With the default no-op tracer provider (tracing disabled) span creation is
// effectively free, so it is safe to wire unconditionally.
type PgxTracer struct {
	Tracer oteltrace.Tracer
}

// NewPgxTracer returns a PgxTracer using the global tracer provider.
func NewPgxTracer() *PgxTracer {
	return &PgxTracer{Tracer: otel.Tracer("pgx")}
}

func (t *PgxTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	stmt := data.SQL
	if len(stmt) > maxStmtLen {
		stmt = stmt[:maxStmtLen]
	}
	ctx, _ = t.Tracer.Start(ctx, "db.query",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.statement", stmt),
		),
	)
	return ctx
}

func (t *PgxTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := oteltrace.SpanFromContext(ctx)
	if data.Err != nil {
		span.SetStatus(codes.Error, data.Err.Error())
	}
	span.End()
}
