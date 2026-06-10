// Package tracing wires OpenTelemetry distributed tracing. It is entirely
// opt-in: when OTEL_EXPORTER_OTLP_ENDPOINT is unset Init is a no-op and the
// global tracer provider stays the default no-op — zero overhead, no behavior
// change. When set, spans for every HTTP request (otelgin) and every DB query
// (PgxTracer) are exported over OTLP/HTTP, and middleware.TraceID reuses the
// active span's trace ID so structured logs correlate with traces.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Init configures the global OTel tracer provider from the environment.
// Returns a shutdown func (flushes pending spans), whether tracing is
// enabled, and any setup error. Disabled (endpoint unset) is not an error.
func Init(ctx context.Context, serviceName, env string) (func(context.Context) error, bool, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, false, nil
	}

	// Endpoint/headers/TLS are read from the standard OTEL_EXPORTER_OTLP_*
	// env vars by the exporter itself.
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, false, err
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		attribute.String("deployment.environment.name", env),
	))
	if err != nil {
		return nil, false, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		// Honor inbound sampling decisions; sample locally-rooted traces.
		// Ratio is tunable via the standard OTEL_TRACES_SAMPLER(_ARG) vars,
		// which the SDK reads; this is the default when they're unset.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)
	otel.SetTracerProvider(tp)
	// W3C traceparent + baggage propagation for inbound/outbound requests.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return tp.Shutdown, true, nil
}
