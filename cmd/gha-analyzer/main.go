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
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	args := os.Args[1:]
	perfettoFile := ""
	openInPerfetto := false
	openInOTel := false
	
	filtered := []string{}
	for _, arg := range args {
		if arg == "help" || arg == "--help" || arg == "-h" {
			printUsage()
			os.Exit(0)
		}
		if strings.HasPrefix(arg, "--perfetto=") {
			perfettoFile = strings.TrimPrefix(arg, "--perfetto=")
			continue
		}
		if arg == "--open-in-perfetto" {
			openInPerfetto = true
			continue
		}
		if arg == "--open-in-otel" {
			openInOTel = true
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
		sdktrace.WithIDGenerator(githubapi.GHIDGenerator{}),
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

	if openInOTel {
		fmt.Println("Opening OTel Desktop Viewer...")
		_ = utils.OpenBrowser("http://localhost:8000")
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
	fmt.Println("GitHub Actions Analyzer")
	fmt.Println("\nUsage:")
	fmt.Println("  gha-analyzer <github_url1> [github_url2...] [token] [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  --perfetto=<file.json>    Save trace for Perfetto.dev analysis")
	fmt.Println("  --open-in-perfetto        Automatically open the generated trace in Perfetto UI")
	fmt.Println("  --open-in-otel            Automatically open the OTel Desktop Viewer")
	fmt.Println("  help, --help, -h          Show this help message")
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  GITHUB_TOKEN              GitHub PAT (alternatively pass as argument)")
	fmt.Println("\nExamples:")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/commit/sha --perfetto=trace.json")
}
