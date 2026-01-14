package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/output"
	"github.com/stefanpenner/gha-analyzer/pkg/tui"
)

func main() {
	args := os.Args[1:]
	perfettoFile := ""
	openInPerfetto := false
	format := "text"

	filtered := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--perfetto=") {
			perfettoFile = strings.TrimPrefix(arg, "--perfetto=")
			continue
		}
		if strings.HasPrefix(arg, "--format=") {
			format = strings.TrimPrefix(arg, "--format=")
			continue
		}
		if arg == "--markdown" {
			format = "markdown"
			continue
		}
		if arg == "--open-in-perfetto" {
			openInPerfetto = true
			continue
		}
		filtered = append(filtered, arg)
	}

	var providedToken string
	if len(filtered) > 0 {
		last := filtered[len(filtered)-1]
		if !strings.HasPrefix(last, "http") {
			providedToken = last
			filtered = filtered[:len(filtered)-1]
		}
	}

	urls := filtered
	if len(urls) == 0 || (providedToken == "" && os.Getenv("GITHUB_TOKEN") == "") {
		if len(urls) == 0 {
			fmt.Fprintln(os.Stderr, "Error: No GitHub URLs provided.")
		}
		if providedToken == "" && os.Getenv("GITHUB_TOKEN") == "" {
			fmt.Fprintln(os.Stderr, "Error: GitHub token is required.")
		}
		printUsage()
		os.Exit(1)
	}

	token := providedToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	context := githubapi.NewContext(token)
	client := githubapi.NewClient(context)

	progress := tui.NewProgress(len(urls), os.Stderr)
	progress.Start()

	results, traceEvents, globalEarliest, globalLatest, errs := analyzer.AnalyzeURLs(urls, client, progress)
	progress.Finish()
	progress.Wait()

	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No workflow runs found for any of the provided URLs")
		os.Exit(1)
	}

	combined := analyzer.CalculateCombinedMetrics(results, sumRuns(results), collectStarts(results), collectEnds(results))
	outputWriter := os.Stderr
	if format == "markdown" || format == "md" {
		outputWriter = os.Stdout
	}
	if format == "markdown" || format == "md" {
		if err := output.OutputCombinedResultsMarkdown(outputWriter, results, combined, traceEvents, globalEarliest, globalLatest, perfettoFile, openInPerfetto); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	if err := output.OutputCombinedResults(outputWriter, results, combined, traceEvents, globalEarliest, globalLatest, perfettoFile, openInPerfetto); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: gha-analyzer <github_url1> [github_url2] ... [token] [--perfetto=<file_name_for_trace.json>] [--open-in-perfetto]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Supported URL formats:")
	fmt.Fprintln(os.Stderr, "  PR: https://github.com/owner/repo/pull/123")
	fmt.Fprintln(os.Stderr, "  Commit: https://github.com/owner/repo/commit/abc123...")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  Single URL: gha-analyzer https://github.com/owner/repo/pull/123")
	fmt.Fprintln(os.Stderr, "  Multiple URLs: gha-analyzer https://github.com/owner/repo/pull/123 https://github.com/owner/repo/commit/abc123")
	fmt.Fprintln(os.Stderr, "  With token: gha-analyzer https://github.com/owner/repo/pull/123 your_token")
	fmt.Fprintln(os.Stderr, "  With perfetto output: gha-analyzer https://github.com/owner/repo/pull/123 --perfetto=trace.json")
	fmt.Fprintln(os.Stderr, "  With token and perfetto: gha-analyzer https://github.com/owner/repo/pull/123 your_token --perfetto=trace.json")
	fmt.Fprintln(os.Stderr, "  Auto-open in perfetto: gha-analyzer https://github.com/owner/repo/pull/123 --perfetto=trace.json --open-in-perfetto")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "GitHub token can be provided as:")
	fmt.Fprintln(os.Stderr, "  1. Command line argument: gha-analyzer <github_urls> <token>")
	fmt.Fprintln(os.Stderr, "  2. Environment variable: export GITHUB_TOKEN=<token>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Perfetto tracing:")
	fmt.Fprintln(os.Stderr, "  Use --perfetto=<filename> to save Chrome Tracing format output to a file")
	fmt.Fprintln(os.Stderr, "  Use --open-in-perfetto to automatically open the trace in Perfetto UI")
	fmt.Fprintln(os.Stderr, "  If not specified, no trace file will be generated")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Output format:")
	fmt.Fprintln(os.Stderr, "  Use --format=markdown (or --markdown) for Markdown output")
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
