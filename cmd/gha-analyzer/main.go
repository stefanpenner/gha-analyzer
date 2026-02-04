package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/core"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	perfettoexport "github.com/stefanpenner/gha-analyzer/pkg/export/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/export/terminal"
	"github.com/stefanpenner/gha-analyzer/pkg/perfetto"
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

// reloadProgressAdapter adapts tuiresults.LoadingReporter to analyzer.ProgressReporter
type reloadProgressAdapter struct {
	reporter tuiresults.LoadingReporter
}

func (a *reloadProgressAdapter) StartURL(urlIndex int, url string) {
	if a.reporter != nil {
		a.reporter.SetURL(url)
	}
}

func (a *reloadProgressAdapter) SetURLRuns(runCount int) {
	// Not directly reportable to LoadingReporter
}

func (a *reloadProgressAdapter) SetPhase(phase string) {
	if a.reporter != nil {
		a.reporter.SetPhase(phase)
	}
}

func (a *reloadProgressAdapter) SetDetail(detail string) {
	if a.reporter != nil {
		a.reporter.SetDetail(detail)
	}
}

func (a *reloadProgressAdapter) ProcessRun() {
	// Not directly reportable to LoadingReporter
}

func (a *reloadProgressAdapter) Finish() {
	// Not directly reportable to LoadingReporter
}

func printError(err error, context string) {
	// Print the full error message, not just flattened
	fmt.Fprintf(os.Stderr, "%sError: %s: %v%s\n", colorRed, context, err, colorReset)
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
	tuiMode := isTerminal() // TUI only enabled if running in a terminal
	clearCache := false
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
		if arg == "--no-tui" || arg == "--notui" {
			tuiMode = false
			continue
		}
		if arg == "--clear-cache" {
			clearCache = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered

	// Handle --clear-cache flag
	if clearCache {
		cacheDir := githubapi.DefaultCacheDir()
		if err := os.RemoveAll(cacheDir); err != nil {
			printError(err, "failed to clear cache")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Cache cleared: %s\n", cacheDir)
		if len(args) == 0 {
			os.Exit(0)
		}
	}

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
		exporters = append(exporters, perfettoexport.NewExporter(os.Stderr, perfettoFile, openInPerfetto))
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
		// Handle perfetto export before TUI starts (so it opens immediately)
		if perfettoFile != "" {
			combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
			var allTraceEvents []analyzer.TraceEvent
			for _, res := range results {
				allTraceEvents = append(allTraceEvents, res.TraceEvents...)
			}
			if err := perfetto.WriteTrace(os.Stderr, results, combined, allTraceEvents, globalEarliest, perfettoFile, openInPerfetto, spans); err != nil {
				printError(err, "writing perfetto trace failed")
			}
		}

		globalStartTime := time.UnixMilli(globalEarliest)
		globalEndTime := time.UnixMilli(globalLatest)

		// Create reload function that clears cache and refetches data
		reloadFunc := func(reporter tuiresults.LoadingReporter) ([]sdktrace.ReadOnlySpan, time.Time, time.Time, error) {
			// Report progress if reporter is available
			if reporter != nil {
				reporter.SetPhase("Clearing cache")
			}

			// Clear cache
			if err := os.RemoveAll(githubapi.DefaultCacheDir()); err != nil {
				return nil, time.Time{}, time.Time{}, fmt.Errorf("failed to clear cache: %w", err)
			}

			if reporter != nil {
				reporter.SetPhase("Setting up")
			}

			// Create new collector and tracer provider for reload
			reloadCollector := core.NewSpanCollector()
			reloadRes, _ := otelexport.GetResource(ctx)
			reloadTP := sdktrace.NewTracerProvider(
				sdktrace.WithSyncer(reloadCollector), // Use Syncer instead of Batcher for immediate export
				sdktrace.WithResource(reloadRes),
				sdktrace.WithIDGenerator(githubapi.GHIDGenerator{}),
			)
			otel.SetTracerProvider(reloadTP)

			// Create a progress reporter adapter if we have one
			var progressReporter analyzer.ProgressReporter
			if reporter != nil {
				progressReporter = &reloadProgressAdapter{reporter: reporter}
			}

			// Re-run ingestion
			reloadClient := githubapi.NewClient(githubapi.NewContext(token))
			reloadIngestor := polling.NewPollingIngestor(reloadClient, args, progressReporter, analyzer.AnalyzeOptions{
				Window: window,
			})
			_, reloadEarliest, reloadLatest, err := reloadIngestor.Ingest(ctx)
			if err != nil {
				reloadTP.Shutdown(ctx)
				return nil, time.Time{}, time.Time{}, err
			}

			if reporter != nil {
				reporter.SetPhase("Finalizing")
			}

			// Force flush and collect spans before shutdown
			reloadTP.ForceFlush(ctx)
			reloadSpans := reloadCollector.Spans()
			reloadTP.Shutdown(ctx)

			return reloadSpans, time.UnixMilli(reloadEarliest), time.UnixMilli(reloadLatest), nil
		}

		// Create function to open in Perfetto from TUI
		openPerfettoFunc := func() {
			// Create temp file for perfetto trace
			tmpFile, err := os.CreateTemp("", "gha-trace-*.json")
			if err != nil {
				return
			}
			tmpFile.Close()

			combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
			var allTraceEvents []analyzer.TraceEvent
			for _, res := range results {
				allTraceEvents = append(allTraceEvents, res.TraceEvents...)
			}
			_ = perfetto.WriteTrace(io.Discard, results, combined, allTraceEvents, globalEarliest, tmpFile.Name(), true, spans)
		}

		if err := tuiresults.Run(spans, globalStartTime, globalEndTime, args, reloadFunc, openPerfettoFunc); err != nil {
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
	fmt.Println("  --tui                     Force interactive TUI mode (default when terminal is available)")
	fmt.Println("  --no-tui                  Disable interactive TUI, use CLI output instead")
	fmt.Println("  --perfetto=<file.json>    Save trace for Perfetto.dev analysis")
	fmt.Println("  --open-in-perfetto        Automatically open the generated trace in Perfetto UI")
	fmt.Println("  --otel[=<endpoint>]       Export traces to OTel collector (default: localhost:4318)")
	fmt.Println("  --open-in-otel            Automatically open the OTel Desktop Viewer")
	fmt.Println("  --window=<duration>       Only show events within <duration> of merge/latest activity (e.g. 24h, 2h)")
	fmt.Println("  --clear-cache             Clear the HTTP cache (can be combined with other flags)")
	fmt.Println("  help, --help, -h          Show this help message")
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  GITHUB_TOKEN              GitHub PAT (alternatively pass as argument)")
	fmt.Println("\nExamples:")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/commit/sha --perfetto=trace.json")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123 --no-tui")
	fmt.Println("  gha-analyzer --clear-cache")
}

// isTerminal checks if stdout and stderr are connected to a terminal
func isTerminal() bool {
	// Check if stdout is a terminal using file mode
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
