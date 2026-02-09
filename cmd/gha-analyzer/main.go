package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/core"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	perfettoexport "github.com/stefanpenner/gha-analyzer/pkg/export/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/export/terminal"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/ingest/polling"
	"github.com/stefanpenner/gha-analyzer/pkg/ingest/webhook"
	"github.com/stefanpenner/gha-analyzer/pkg/output"
	"github.com/stefanpenner/gha-analyzer/pkg/perfetto"
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

type config struct {
	urls             []string
	perfettoFile     string
	openInPerfetto   bool
	openInOTel       bool
	otelEndpoint     string
	otelStdout       bool
	otelGRPCEndpoint string
	tuiMode          bool
	clearCache       bool
	window           time.Duration
	showHelp         bool
	trendsMode       bool
	trendsRepo       string
	trendsDays       int
	trendsFormat     string
	trendsBranch     string
	trendsWorkflow   string
}

func parseArgs(args []string, terminal bool) (config, error) {
	cfg := config{
		tuiMode:      terminal,
		trendsDays:   30, // default to 30 days
		trendsFormat: "terminal",
	}

	// Check if first arg is "trends" subcommand
	if len(args) > 0 && args[0] == "trends" {
		cfg.trendsMode = true
		args = args[1:] // consume the "trends" subcommand
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "help" || arg == "--help" || arg == "-h" {
			cfg.showHelp = true
			continue
		}
		if strings.HasPrefix(arg, "--perfetto=") {
			cfg.perfettoFile = strings.TrimPrefix(arg, "--perfetto=")
			continue
		}
		if strings.HasPrefix(arg, "--window=") {
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--window="))
			if err != nil {
				return cfg, fmt.Errorf("invalid window duration %s: %w", arg, err)
			}
			cfg.window = d
			continue
		}
		if arg == "--open-in-perfetto" {
			cfg.openInPerfetto = true
			continue
		}
		if arg == "--open-in-otel" {
			cfg.openInOTel = true
			continue
		}
		if strings.HasPrefix(arg, "--otel=") {
			cfg.otelEndpoint = strings.TrimPrefix(arg, "--otel=")
			continue
		}
		if arg == "--otel" {
			cfg.otelStdout = true
			continue
		}
		if strings.HasPrefix(arg, "--otel-grpc=") {
			cfg.otelGRPCEndpoint = strings.TrimPrefix(arg, "--otel-grpc=")
			continue
		}
		if arg == "--otel-grpc" {
			cfg.otelGRPCEndpoint = "localhost:4317"
			continue
		}
		if arg == "--tui" {
			cfg.tuiMode = true
			continue
		}
		if arg == "--no-tui" || arg == "--notui" {
			cfg.tuiMode = false
			continue
		}
		if arg == "--clear-cache" {
			cfg.clearCache = true
			continue
		}

		// Trends-specific flags
		if strings.HasPrefix(arg, "--days=") {
			days := strings.TrimPrefix(arg, "--days=")
			var err error
			_, err = fmt.Sscanf(days, "%d", &cfg.trendsDays)
			if err != nil || cfg.trendsDays < 1 {
				return cfg, fmt.Errorf("invalid --days value: %s", days)
			}
			continue
		}
		if strings.HasPrefix(arg, "--format=") {
			cfg.trendsFormat = strings.TrimPrefix(arg, "--format=")
			if cfg.trendsFormat != "terminal" && cfg.trendsFormat != "json" {
				return cfg, fmt.Errorf("invalid --format value: %s (must be 'terminal' or 'json')", cfg.trendsFormat)
			}
			continue
		}
		if strings.HasPrefix(arg, "--branch=") {
			cfg.trendsBranch = strings.TrimPrefix(arg, "--branch=")
			continue
		}
		if strings.HasPrefix(arg, "--workflow=") {
			cfg.trendsWorkflow = strings.TrimPrefix(arg, "--workflow=")
			continue
		}

		// For trends mode, first non-flag arg is the repo
		if cfg.trendsMode && cfg.trendsRepo == "" && !strings.HasPrefix(arg, "-") {
			cfg.trendsRepo = arg
			continue
		}

		cfg.urls = append(cfg.urls, arg)
	}

	return cfg, nil
}

func main() {
	cfg, err := parseArgs(os.Args[1:], isTerminal())
	if err != nil {
		printErrorMsg(err.Error())
		os.Exit(1)
	}

	if cfg.showHelp {
		printUsage()
		os.Exit(0)
	}

	args := cfg.urls

	// Handle --clear-cache flag
	if cfg.clearCache {
		cacheDir := githubapi.DefaultCacheDir()
		if err := os.RemoveAll(cacheDir); err != nil {
			printError(err, "failed to clear cache")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Cache cleared: %s\n", cacheDir)
		if len(args) == 0 && !cfg.trendsMode {
			os.Exit(0)
		}
	}

	// Handle trends mode
	if cfg.trendsMode {
		if cfg.trendsRepo == "" {
			printErrorMsg("Trends mode requires a repository in format 'owner/repo'\n\n  Usage: gha-analyzer trends owner/repo [--days=30] [--format=terminal|json]\n\n  Run 'gha-analyzer --help' for more information.")
			os.Exit(1)
		}

		// Parse owner/repo
		parts := strings.Split(cfg.trendsRepo, "/")
		if len(parts) != 2 {
			printErrorMsg(fmt.Sprintf("Invalid repository format: %s (expected 'owner/repo')", cfg.trendsRepo))
			os.Exit(1)
		}
		owner, repo := parts[0], parts[1]

		// Get GitHub token
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			if ghPath, err := exec.LookPath("gh"); err == nil {
				if out, err := exec.Command(ghPath, "auth", "token").Output(); err == nil {
					token = strings.TrimSpace(string(out))
				}
			}
		}
		if token == "" {
			printErrorMsg("GITHUB_TOKEN environment variable is required.\n  Tip: install the GitHub CLI (gh) and run `gh auth login` to authenticate automatically.")
			os.Exit(1)
		}

		ctx := context.Background()
		client := githubapi.NewClient(githubapi.NewContext(token))

		// Build filter description for user feedback
		filterDesc := fmt.Sprintf("Analyzing trends for %s/%s over the last %d days", owner, repo, cfg.trendsDays)
		if cfg.trendsBranch != "" {
			filterDesc += fmt.Sprintf(" (branch: %s)", cfg.trendsBranch)
		}
		if cfg.trendsWorkflow != "" {
			filterDesc += fmt.Sprintf(" (workflow: %s)", cfg.trendsWorkflow)
		}
		fmt.Fprintf(os.Stderr, "%s...\n", filterDesc)

		// Perform trend analysis
		analysis, err := analyzer.AnalyzeTrends(ctx, client, owner, repo, cfg.trendsDays, cfg.trendsBranch, cfg.trendsWorkflow)
		if err != nil {
			printError(err, "trend analysis failed")
			os.Exit(1)
		}

		// Output results
		if err := output.OutputTrends(os.Stderr, analysis, cfg.trendsFormat); err != nil {
			printError(err, "output failed")
			os.Exit(1)
		}

		return
	}

	// If no URL args and stdin is piped, read webhook from stdin
	if len(args) == 0 && !isStdinTerminal() {
		fmt.Fprintf(os.Stderr, "Reading webhook from stdin...\n")
		urls, err := webhook.ParseWebhook(os.Stdin)
		if err != nil {
			printError(err, "failed to parse webhook")
			os.Exit(1)
		}
		args = urls
	}

	if len(args) == 0 {
		printErrorMsg("No GitHub URLs provided.\n\n  Usage: gha-analyzer <github_url> [flags]\n\n  Run 'gha-analyzer --help' for more information.")
		os.Exit(1)
	}

	// When --otel stdout is used, disable TUI so output goes to stdout cleanly
	if cfg.otelStdout {
		cfg.tuiMode = false
	}

	perfettoFile := cfg.perfettoFile

	// Auto-generate perfetto file if --open-in-perfetto is used without --perfetto
	if cfg.openInPerfetto && perfettoFile == "" {
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
				// Skip arguments that look like GitHub URLs (shorthand or full)
				if _, err := utils.ParseGitHubURL(arg); err == nil {
					continue
				}
				token = arg
				args = append(args[:i], args[i+1:]...)
				break
			}
		}
	}

	// Fall back to `gh auth token` if gh CLI is available
	if token == "" {
		if ghPath, err := exec.LookPath("gh"); err == nil {
			if out, err := exec.Command(ghPath, "auth", "token").Output(); err == nil {
				token = strings.TrimSpace(string(out))
			}
		}
	}

	if token == "" {
		printErrorMsg("GITHUB_TOKEN environment variable or token argument is required.\n  Tip: install the GitHub CLI (gh) and run `gh auth login` to authenticate automatically.")
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
		exporters = append(exporters, perfettoexport.NewExporter(os.Stderr, perfettoFile, cfg.openInPerfetto))
	}

	if cfg.otelStdout {
		stdoutExporter, err := otelexport.NewStdoutExporter(os.Stdout)
		if err == nil {
			exporters = append(exporters, stdoutExporter)
		}
	}

	if cfg.otelEndpoint != "" {
		otelExporter, err := otelexport.NewExporter(ctx, cfg.otelEndpoint)
		if err == nil {
			exporters = append(exporters, otelExporter)
		}
	}

	if cfg.otelGRPCEndpoint != "" {
		grpcExporter, err := otelexport.NewGRPCExporter(ctx, cfg.otelGRPCEndpoint)
		if err == nil {
			exporters = append(exporters, grpcExporter)
		}
	}

	pipeline := core.NewPipeline(exporters...)

	// 4. Setup Progress TUI
	progress := tui.NewProgress(len(args), os.Stderr)
	progress.Start()

	// 5. Run Ingestor
	ingestor := polling.NewPollingIngestor(client, args, progress, analyzer.AnalyzeOptions{
		Window: cfg.window,
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
	if cfg.tuiMode {
		// Handle perfetto export before TUI starts (so it opens immediately)
		if perfettoFile != "" {
			combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
			var allTraceEvents []analyzer.TraceEvent
			for _, res := range results {
				allTraceEvents = append(allTraceEvents, res.TraceEvents...)
			}
			if err := perfetto.WriteTrace(os.Stderr, results, combined, allTraceEvents, globalEarliest, perfettoFile, cfg.openInPerfetto, spans); err != nil {
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
				Window: cfg.window,
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
	output.OutputCombinedResults(os.Stderr, results, combined, allTraceEvents, globalEarliest, globalLatest, perfettoFile, cfg.openInPerfetto, spans)

	if err := pipeline.Finish(ctx); err != nil {
		printError(err, "finalizing pipeline failed")
	}

	if cfg.openInOTel {
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
	fmt.Println("  gha-analyzer trends <owner/repo> [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  --tui                     Force interactive TUI mode (default when terminal is available)")
	fmt.Println("  --no-tui                  Disable interactive TUI, use CLI output instead")
	fmt.Println("  --perfetto=<file.json>    Save trace for Perfetto.dev analysis")
	fmt.Println("  --open-in-perfetto        Automatically open the generated trace in Perfetto UI")
	fmt.Println("  --otel                    Write OTel spans as JSON to stdout")
	fmt.Println("  --otel=<endpoint>         Export traces via OTLP/HTTP (default port: 4318)")
	fmt.Println("  --otel-grpc[=<endpoint>]  Export traces via OTLP/gRPC (default: localhost:4317)")
	fmt.Println("  --open-in-otel            Automatically open the OTel Desktop Viewer")
	fmt.Println("  --window=<duration>       Only show events within <duration> of merge/latest activity (e.g. 24h, 2h)")
	fmt.Println("  --clear-cache             Clear the HTTP cache (can be combined with other flags)")
	fmt.Println("  help, --help, -h          Show this help message")
	fmt.Println("\nTrends Mode Flags:")
	fmt.Println("  --days=<n>                Number of days to analyze (default: 30)")
	fmt.Println("  --format=<format>         Output format: 'terminal' or 'json' (default: terminal)")
	fmt.Println("  --branch=<name>           Filter by branch name (e.g., main, master)")
	fmt.Println("  --workflow=<file>         Filter by workflow file name (e.g., post-merge.yaml)")
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  GITHUB_TOKEN              GitHub PAT (alternatively pass as argument)")
	fmt.Println("\nExamples:")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/commit/sha --perfetto=trace.json")
	fmt.Println("  gha-analyzer https://github.com/owner/repo/pull/123 --no-tui")
	fmt.Println("  gha-analyzer trends owner/repo")
	fmt.Println("  gha-analyzer trends owner/repo --days=7 --format=json")
	fmt.Println("  gha-analyzer trends owner/repo --branch=main --workflow=post-merge.yaml")
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

// isStdinTerminal checks if stdin is connected to a terminal
func isStdinTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
