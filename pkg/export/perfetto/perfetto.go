package perfetto

import (
	"context"
	"io"
	"sync"

	"go.opentelemetry.io/otel/sdk/trace"
)

type Exporter struct {
	writer         io.Writer
	filename       string
	openInPerfetto bool
	spans          []trace.ReadOnlySpan
	mu             sync.Mutex
}

func NewExporter(w io.Writer, filename string, openInPerfetto bool) *Exporter {
	return &Exporter{
		writer:         w,
		filename:       filename,
		openInPerfetto: openInPerfetto,
	}
}

func (e *Exporter) Export(ctx context.Context, spans []trace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *Exporter) Finish(ctx context.Context) error {
	// Stub
	return nil
}
