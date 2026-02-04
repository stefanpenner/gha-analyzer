package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/core"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	"github.com/stefanpenner/gha-analyzer/pkg/export/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/export/terminal"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/ingest/polling"
	"github.com/stefanpenner/gha-analyzer/pkg/output"
	"github.com/stefanpenner/gha-analyzer/pkg/tui"
	tuiresults "github.com/stefanpenner/gha-analyzer/pkg/tui/results"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ANSI color codes
const (
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

func printError(err error, context string) {
	// Use cockroachdb/errors to get a clean, user-friendly message
	msg := errors.FlattenDetails(err)
	fmt.Fprintf(os.Stderr, "%sError: %s: %s%s\n", colorRed, context, msg, colorReset)
}

func printErrorMsg(message string) {
	fmt.Fprintf(os.Stderr, "%sError: %s%s\n", colorRed, message, colorReset)
}

func main() {
	args := os.Args[1:]
	perfettoFile := ""
	openInPerfetto := false
	openInOTel := false
	otelEndpoint := ""
	tuiMode := false
	var window time.Duration

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
		if strings.HasPrefix(arg, "--window=") {
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--window="))
			if err != nil {
				printError(err, fmt.Sprintf("invalid window duration %s", arg))
				os.Exit(1)
			}
			window = d
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
		if strings.HasPrefix(arg, "--otel=") {
			otelEndpoint = strings.TrimPrefix(arg, "--otel=")
			continue
		}
		if arg == "--otel" {
			otelEndpoint = "localhost:4318"
			continue
		}
		if arg == "--tui" {
			tuiMode = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered

	// Auto-generate perfetto file if --open-in-perfetto is used without --perfetto
	if openInPerfetto && perfettoFile == "" {
		tmpFile, err := os.CreateTemp("", "gha-trace-*.json")
		if err == nil {
			perfettoFile = tmpFile.Name()
			tmpFile.Close()
		}
	}

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
		printErrorMsg("GITHUB_TOKEN environment variable or token argument is required")
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

	if otelEndpoint != "" {
		otelExporter, err := otelexport.NewExporter(ctx, otelEndpoint)
		if err == nil {
			exporters = append(exporters, otelExporter)
		}
	}

	pipeline := core.NewPipeline(exporters...)

	// 4. Setup Progress TUI
	progress := tui.NewProgress(len(args), os.Stderr)
	progress.Start()

	// 5. Run Ingestor
	ingestor := polling.NewPollingIngestor(client, args, progress, analyzer.AnalyzeOptions{
		Window: window,
	})
	results, globalEarliest, globalLatest, err := ingestor.Ingest(ctx)
	
	progress.Finish()
	progress.Wait()

	if err != nil {
		printError(err, "ingestion failed")
		os.Exit(1)
	}

	// 6. Finalize & Process Spans
	tp.ForceFlush(ctx)
	spans := collector.Spans()

	if err := pipeline.Process(ctx, spans); err != nil {
		printError(err, "processing spans failed")
	}

	// If TUI mode is enabled, launch interactive TUI
	if tuiMode {
		globalStartTime := time.UnixMilli(globalEarliest)
		globalEndTime := time.UnixMilli(globalLatest)
		if err := tuiresults.Run(spans, globalStartTime, globalEndTime, args); err != nil {
			fmt.Fprintf(os.Stderr, "%sError: TUI failed: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		return
	}

	// Restore rich CLI report
	combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
	var allTraceEvents []analyzer.TraceEvent
	for _, res := range results {
		allTraceEvents = append(allTraceEvents, res.TraceEvents...)
	}
	output.OutputCombinedResults(os.Stderr, results, combined, allTraceEvents, globalEarliest, globalLatest, perfettoFile, openInPerfetto, spans)

	if err := pipeline.Finish(ctx); err != nil {
		printError(err, "finalizing pipeline failed")
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
	fmt.Println("  --tui                     Launch interactive TUI with expandable workflow tree")
	fmt.Println("  --perfetto=<file.json>    Save trace for Perfetto.dev analysis")
	fmt.Println("  --open-in-perfetto        Automatically open the generated trace in Perfetto UI")
	fmt.Println("  --otel[=<endpoint>]       Export traces to OTel collector (default: localhost:4318)")
	fmt.Println("  --open-in-otel            Automatically open the OTel Desktop Viewer")
	fmt.Println("  --window=<duration>       Only show events within <duration> of merge/latest activity (e.g. 24h, 2h)")
	fmt.Println("  help, --help, -h          Show this help message")
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  GITHUB_TOKEN              GitHub PAT (alternatively pass as argument)")
	fmt.Println("\nExamples:")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/commit/sha --perfetto=trace.json")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123 --tui")
}
