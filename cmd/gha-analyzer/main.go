package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	"github.com/stefanpenner/gha-analyzer/pkg/export/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/export/terminal"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/ingest/polling"
	"github.com/stefanpenner/gha-analyzer/pkg/output"
	"github.com/stefanpenner/gha-analyzer/pkg/tui"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	args := os.Args[1:]
	perfettoFile := ""
	openInPerfetto := false
	
	filtered := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--perfetto=") {
			perfettoFile = strings.TrimPrefix(arg, "--perfetto=")
			continue
		}
		if arg == "--open-in-perfetto" {
			openInPerfetto = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered

	// 1. Setup GitHub Token
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		for i, arg := range args {
			if !strings.HasPrefix(arg, "http") && !strings.HasPrefix(arg, "-") {
				token = arg
				args = append(args[:i], args[i+1:]...)
				break
			}
		}
	}

	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: GITHUB_TOKEN environment variable or token argument is required")
		printUsage()
		os.Exit(1)
	}

	ctx := context.Background()

	// 2. Setup OTel
	collector := core.NewSpanCollector()
	res, _ := otelexport.GetResource(ctx)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(collector),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(ctx)

	client := githubapi.NewClient(githubapi.NewContext(token))

	// 3. Setup Exporters
	exporters := []core.Exporter{
		terminal.NewExporter(os.Stderr),
	}

	if perfettoFile != "" {
		exporters = append(exporters, perfetto.NewExporter(os.Stderr, perfettoFile, openInPerfetto))
	}

	otelExporter, err := otelexport.NewExporter(ctx, "localhost:4318") // Using 4318 for HTTP
	if err == nil {
		exporters = append(exporters, otelExporter)
	}

	pipeline := core.NewPipeline(exporters...)

	// 4. Setup Progress TUI
	progress := tui.NewProgress(len(args), os.Stderr)
	progress.Start()

	// 5. Run Ingestor
	ingestor := polling.NewPollingIngestor(client, args, progress)
	results, globalEarliest, globalLatest, err := ingestor.Ingest(ctx)
	
	progress.Finish()
	progress.Wait()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Ingestion failed: %v\n", err)
		os.Exit(1)
	}

	// 6. Finalize & Process Spans
	tp.ForceFlush(ctx)
	spans := collector.Spans()

	if err := pipeline.Process(ctx, spans); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Processing spans failed: %v\n", err)
	}

	// Restore rich CLI report
	combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
	var allTraceEvents []analyzer.TraceEvent
	for _, res := range results {
		allTraceEvents = append(allTraceEvents, res.TraceEvents...)
	}
	output.OutputCombinedResults(os.Stderr, results, combined, allTraceEvents, globalEarliest, globalLatest, perfettoFile, openInPerfetto, spans)

	if err := pipeline.Finish(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Finalizing pipeline failed: %v\n", err)
	}
}

func sumRuns(results []analyzer.URLResult) int {
	total := 0
	for _, result := range results {
		total += result.Metrics.TotalRuns
	}
	return total
}

func collectStarts(results []analyzer.URLResult) []analyzer.JobEvent {
	var events []analyzer.JobEvent
	for _, result := range results {
		events = append(events, result.JobStartTimes...)
	}
	return events
}

func collectEnds(results []analyzer.URLResult) []analyzer.JobEvent {
	var events []analyzer.JobEvent
	for _, result := range results {
		events = append(events, result.JobEndTimes...)
	}
	return events
}

func printUsage() {
	fmt.Println("Usage: ./run.sh cli <github_url1> [token] [--perfetto=<file.json>] [--open-in-perfetto]")
}
