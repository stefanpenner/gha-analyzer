package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

func OutputCombinedResultsMarkdown(w io.Writer, urlResults []analyzer.URLResult, combined analyzer.CombinedMetrics, traceEvents []analyzer.TraceEvent, globalEarliestTime, globalLatestTime int64, perfettoFile string, openInPerfetto bool, spans []trace.ReadOnlySpan) error {
	fmt.Fprintln(w, "# GitHub Actions Performance Report")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "- Perfetto UI: %s\n", markdownLink("https://ui.perfetto.dev", "https://ui.perfetto.dev"))
	if perfettoFile != "" {
		fmt.Fprintf(w, "- Perfetto trace: `%s`\n", perfettoFile)
	}
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "## Summary")
	fmt.Fprintf(w, "- URLs: **%d**\n", len(urlResults))
	fmt.Fprintf(w, "- Runs: **%d**\n", combined.TotalRuns)
	fmt.Fprintf(w, "- Jobs: **%d**\n", combined.TotalJobs)
	fmt.Fprintf(w, "- Steps: **%d**\n", combined.TotalSteps)
	fmt.Fprintf(w, "- Success rate: **%s%% workflows**, **%s%% jobs**\n", combined.SuccessRate, combined.JobSuccessRate)
	fmt.Fprintf(w, "- Peak concurrency: **%d**\n", combined.MaxConcurrency)
	fmt.Fprintln(w, "")

	if len(traceEvents) > 0 {
		fmt.Fprintf(w, "- Trace events: **%d**\n", len(traceEvents))
		fmt.Fprintln(w, "")
	}

	pending := collectPending(urlResults)
	if len(pending) > 0 {
		fmt.Fprintln(w, "## Pending Jobs")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "| Job | Status | Source |")
		fmt.Fprintln(w, "| --- | --- | --- |")
		for _, job := range pending {
			fmt.Fprintf(w, "| %s | %s | %s |\n",
				markdownLink(job.URL, job.Name+requiredEmoji(job.IsRequired)),
				job.Status,
				job.SourceName,
			)
		}
		fmt.Fprintln(w, "")
	}

	sortedResults := sortByEarliest(urlResults)
	fmt.Fprintln(w, "## URLs")
	fmt.Fprintln(w, "")
	for _, result := range sortedResults {
		label := result.DisplayName
		if result.Type == "pr" && result.BranchName != "" {
			label = fmt.Sprintf("%s (%s)", result.DisplayName, result.BranchName)
		}
		fmt.Fprintf(w, "- %s\n", markdownLink(result.DisplayURL, label))
	}
	fmt.Fprintln(w, "")

	if len(urlResults) > 1 {
		fmt.Fprintln(w, "## Combined Timeline")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "_Timeline visualization is available in text output._")
		fmt.Fprintln(w, "")
	}

	if len(urlResults) > 0 {
		fmt.Fprintln(w, "## Run Summary")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "| URL | Runs | Wall | Compute | Approvals | Merged |")
		fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | :---: |")
		for _, result := range urlResults {
			wallMs, computeMs := computeTimelineDurations(result.Metrics.JobTimeline)
			approvals := countReviewEvents(result.ReviewEvents, "shippit") + countReviewEvents(result.ReviewEvents, "merged")
			merged := countReviewEvents(result.ReviewEvents, "merged") > 0
			fmt.Fprintf(w, "| %s | %d | %s | %s | %d | %s |\n",
				markdownLink(result.DisplayURL, result.DisplayName),
				result.Metrics.TotalRuns,
				utils.HumanizeTime(float64(wallMs)/1000),
				utils.HumanizeTime(float64(computeMs)/1000),
				approvals,
				boolYesNo(merged),
			)
		}
		fmt.Fprintln(w, "")
	}

	if len(urlResults) > 0 {
		fmt.Fprintln(w, "## Slowest Jobs")
		fmt.Fprintln(w, "")
		grouped := map[string][]analyzer.CombinedTimelineJob{}
		for _, job := range combined.JobTimeline {
			grouped[job.SourceURL] = append(grouped[job.SourceURL], job)
		}

		for _, result := range sortedResults {
			jobs := grouped[result.DisplayURL]
			if len(jobs) == 0 {
				continue
			}
			analyzer.SortCombinedJobsByDuration(jobs)
			if len(jobs) > 5 {
				jobs = jobs[:5]
			}
			fmt.Fprintf(w, "### %s\n", markdownLink(result.DisplayURL, result.DisplayName))
			for _, job := range jobs {
				duration := float64(job.EndTime-job.StartTime) / 1000
				jobText := fmt.Sprintf("%s — %s%s", utils.HumanizeTime(duration), job.Name, requiredEmoji(job.IsRequired))
				if job.URL != "" {
					jobText = markdownLink(job.URL, jobText)
				}
				fmt.Fprintf(w, "- %s\n", jobText)
			}
			fmt.Fprintln(w, "")
		}
	}

	if perfettoFile != "" {
		if err := perfetto.WriteTrace(w, urlResults, combined, traceEvents, globalEarliestTime, perfettoFile, openInPerfetto, spans); err != nil {
			return err
		}
	}
	return nil
}

func markdownLink(url, text string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

func collectPending(results []analyzer.URLResult) []PendingJobWithSource {
	allPending := []PendingJobWithSource{}
	for _, result := range results {
		for _, job := range result.Metrics.PendingJobs {
			allPending = append(allPending, PendingJobWithSource{
				PendingJob: job,
				SourceURL:  result.DisplayURL,
				SourceName: result.DisplayName,
			})
		}
	}
	sort.Slice(allPending, func(i, j int) bool {
		return strings.Compare(allPending[i].Name, allPending[j].Name) < 0
	})
	return allPending
}

func computeTimelineDurations(timeline []analyzer.TimelineJob) (int64, int64) {
	if len(timeline) == 0 {
		return 0, 0
	}
	start := timeline[0].StartTime
	end := timeline[0].EndTime
	computeMs := int64(0)
	for _, job := range timeline {
		if job.StartTime < start {
			start = job.StartTime
		}
		if job.EndTime > end {
			end = job.EndTime
		}
		if job.EndTime > job.StartTime {
			computeMs += job.EndTime - job.StartTime
		}
	}
	return maxInt64(0, end-start), computeMs
}
