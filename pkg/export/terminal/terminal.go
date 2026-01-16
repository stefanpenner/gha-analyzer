package terminal

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Exporter struct {
	writer io.Writer
	spans  []trace.ReadOnlySpan
	mu     sync.Mutex
}

func NewExporter(w io.Writer) *Exporter {
	return &Exporter{
		writer: io.Discard, // Suppress standard OTel summary, we use the rich report instead
	}
}

func (e *Exporter) Export(ctx context.Context, spans []trace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *Exporter) Finish(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.spans) == 0 {
		return nil
	}

	summary := analyzer.CalculateSummary(e.spans)
	
	fmt.Fprintf(e.writer, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(e.writer, "📊 GitHub Actions Performance Report (OTel-native)\n")
	fmt.Fprintf(e.writer, "%s\n", strings.Repeat("=", 80))

	fmt.Fprintf(e.writer, "\nSummary\n-------\n")
	fmt.Fprintf(e.writer, "Total Runs:      %d\n", summary.TotalRuns)
	fmt.Fprintf(e.writer, "Success Rate:    %.1f%%\n", float64(summary.SuccessfulRuns)/float64(summary.TotalRuns)*100)
	fmt.Fprintf(e.writer, "Total Jobs:      %d\n", summary.TotalJobs)
	fmt.Fprintf(e.writer, "Failed Jobs:     %d\n", summary.FailedJobs)
	fmt.Fprintf(e.writer, "Max Concurrency: %d\n", summary.MaxConcurrency)
	fmt.Fprintf(e.writer, "-------------------------------\n")

	return nil
}
