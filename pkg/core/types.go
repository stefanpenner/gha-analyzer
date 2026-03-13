package core

import (
	"context"

	"go.opentelemetry.io/otel/sdk/trace"
)

// Exporter interface for GHA analysis results.
type Exporter interface {
	Export(ctx context.Context, spans []trace.ReadOnlySpan) error
	Finish(ctx context.Context) error
}
