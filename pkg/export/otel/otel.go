package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

type Exporter struct {
	exporter sdktrace.SpanExporter
}

func NewExporter(ctx context.Context, endpoint string) (*Exporter, error) {
	// Using OTLP/HTTP as it's more compatible with otel-desktop-viewer by default
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	return &Exporter{exporter: exporter}, nil
}

func (e *Exporter) Export(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return e.exporter.ExportSpans(ctx, spans)
}

func (e *Exporter) Finish(ctx context.Context) error {
	return e.exporter.Shutdown(ctx)
}

// GetResource returns a standard resource for GHA analyzer
func GetResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("gha-analyzer"),
		),
	)
}
