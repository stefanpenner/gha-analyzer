package core

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/sdk/trace"
)

// ...

// SpanCollector is an OTel SpanExporter that collects spans in memory.
type SpanCollector struct {
	mu    sync.Mutex
	spans []trace.ReadOnlySpan
}

func NewSpanCollector() *SpanCollector {
	return &SpanCollector{}
}

func (c *SpanCollector) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spans = append(c.spans, spans...)
	return nil
}

func (c *SpanCollector) Shutdown(ctx context.Context) error {
	return nil
}

func (c *SpanCollector) Spans() []trace.ReadOnlySpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.spans
}

func (c *SpanCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spans = nil
}

// SpanType identifies the level of a span in the GHA hierarchy.
type SpanType string

const (
	SpanTypeWorkflow SpanType = "workflow"
	SpanTypeJob      SpanType = "job"
	SpanTypeStep     SpanType = "step"
)

// Exporter interface for GHA analysis results.
// We align this with OTel's SpanExporter but keep it simplified for our CLI needs.
type Exporter interface {
	Export(ctx context.Context, spans []trace.ReadOnlySpan) error
	Finish(ctx context.Context) error
}
