package core

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/sdk/trace"
)

// Pipeline orchestrates the flow of spans from ingestors to exporters.
type Pipeline struct {
	exporters []Exporter
	mu        sync.RWMutex
}

func NewPipeline(exporters ...Exporter) *Pipeline {
	return &Pipeline{
		exporters: exporters,
	}
}

func (p *Pipeline) AddExporter(e Exporter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exporters = append(p.exporters, e)
}

// Process broadcasts a batch of spans to all registered exporters.
func (p *Pipeline) Process(ctx context.Context, spans []trace.ReadOnlySpan) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var wg sync.WaitGroup
	errs := make(chan error, len(p.exporters))

	for _, exporter := range p.exporters {
		wg.Add(1)
		go func(e Exporter) {
			defer wg.Done()
			if err := e.Export(ctx, spans); err != nil {
				errs <- fmt.Errorf("exporter error: %w", err)
			}
		}(exporter)
	}

	wg.Wait()
	close(errs)

	if len(errs) > 0 {
		return <-errs
	}

	return nil
}

func (p *Pipeline) Finish(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, exporter := range p.exporters {
		if err := exporter.Finish(ctx); err != nil {
			return err
		}
	}
	return nil
}
