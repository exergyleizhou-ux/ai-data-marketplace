package tracing

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInit_DisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, enabled, err := Init(context.Background(), "test-svc", "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if enabled {
		t.Fatal("tracing must be disabled when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown must not error: %v", err)
	}
}

func TestInit_EnabledWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	shutdown, enabled, err := Init(context.Background(), "test-svc", "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !enabled {
		t.Fatal("tracing must be enabled when endpoint is set")
	}
	// The exporter endpoint doesn't exist; shutdown must still return (export
	// errors are not fatal) within the context deadline.
	_ = shutdown(context.Background())
	// Restore the default global provider so other tests aren't affected.
	otel.SetTracerProvider(otel.GetTracerProvider())
}

func TestPgxTracer_RecordsQuerySpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tr := &PgxTracer{Tracer: tp.Tracer("test")}

	ctx := tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT id FROM users WHERE id=$1"})
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	got := spans[0]
	if got.Name() != "db.query" {
		t.Errorf("span name = %q, want db.query", got.Name())
	}
	var stmt string
	for _, a := range got.Attributes() {
		if string(a.Key) == "db.statement" {
			stmt = a.Value.AsString()
		}
	}
	if stmt != "SELECT id FROM users WHERE id=$1" {
		t.Errorf("db.statement = %q", stmt)
	}
}

func TestPgxTracer_TruncatesLongStatements(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tr := &PgxTracer{Tracer: tp.Tracer("test")}

	long := "SELECT " + strings.Repeat("x", 200)
	ctx := tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: long})
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	for _, a := range spans[0].Attributes() {
		if string(a.Key) == "db.statement" {
			if n := len(a.Value.AsString()); n > maxStmtLen {
				t.Errorf("db.statement len = %d, want <= %d", n, maxStmtLen)
			}
		}
	}
}
