package analyzer

import (
	"context"

	"go.opentelemetry.io/otel/sdk/trace"
)

// AnalyzerProcessor implements sdktrace.SpanProcessor to calculate GHA metrics.
type AnalyzerProcessor struct {
}

func NewAnalyzerProcessor() *AnalyzerProcessor {
	return &AnalyzerProcessor{}
}

func (p *AnalyzerProcessor) OnStart(parent context.Context, s trace.ReadWriteSpan) {}

func (p *AnalyzerProcessor) OnEnd(s trace.ReadOnlySpan) {}

func (p *AnalyzerProcessor) Shutdown(ctx context.Context) error { return nil }
func (p *AnalyzerProcessor) ForceFlush(ctx context.Context) error { return nil }

// EnrichSpans analyzes a batch of spans and adds concurrency/metric attributes.
func (p *AnalyzerProcessor) EnrichSpans(spans []trace.ReadOnlySpan) {
	// Group spans by workflow run (TraceID)
	// For each group, calculate concurrency, etc.
}
