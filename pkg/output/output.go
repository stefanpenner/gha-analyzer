package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/perfetto"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

type PendingJobWithSource struct {
	analyzer.PendingJob
	SourceURL  string
	SourceName string
}

type CommitAggregate struct {
	Name                    string
	URLIndex                int
	TotalRunsForCommit      int
	TotalComputeMsForCommit int64
}

func OutputCombinedResults(w io.Writer, urlResults []analyzer.URLResult, combined analyzer.CombinedMetrics, traceEvents []analyzer.TraceEvent, globalEarliestTime, globalLatestTime int64, perfettoFile string, openInPerfetto bool, spans []trace.ReadOnlySpan) error {
	if perfettoFile != "" {
		fmt.Fprintf(w, "\n✅ Generated %d trace events • Open in Perfetto.dev for analysis\n", len(traceEvents))
	} else {
		fmt.Fprintf(w, "\n✅ Generated %d trace events • Use --perfetto=<filename> to save trace for Perfetto.dev analysis\n", len(traceEvents))
	}

	fmt.Fprintf(w, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(w, "📊 %s\n", utils.MakeClickableLink("https://ui.perfetto.dev", "GitHub Actions Performance Report - Multi-URL Analysis"))
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 80))

	section(w, "Summary")
	fmt.Fprintf(w, "Analysis Summary: %d URLs • %d runs • %d jobs • %d steps\n", len(urlResults), combined.TotalRuns, combined.TotalJobs, combined.TotalSteps)
	fmt.Fprintf(w, "Success Rate: %s%% workflows, %s%% jobs • Peak Concurrency: %d\n", combined.SuccessRate, combined.JobSuccessRate, combined.MaxConcurrency)

	allPending := []PendingJobWithSource{}
	for _, result := range urlResults {
		for _, job := range result.Metrics.PendingJobs {
			allPending = append(allPending, PendingJobWithSource{
				PendingJob: job,
				SourceURL:  result.DisplayURL,
				SourceName: result.DisplayName,
			})
		}
	}

	if len(allPending) > 0 {
		section(w, "Pending Jobs")
		fmt.Fprintf(w, "%s %d jobs still running\n", utils.BlueText("⚠️  Pending Jobs Detected:"), len(allPending))
		for i, job := range allPending {
			jobLink := utils.MakeClickableLink(job.URL, job.Name)
			fmt.Fprintf(w, "  %d. %s (%s) - %s\n", i+1, utils.BlueText(jobLink), job.Status, job.SourceName)
		}
		fmt.Fprintf(w, "\n  Note: Timeline shows current progress for pending jobs. Results may change as jobs complete.\n")
	}

	sortedResults := sortByEarliest(urlResults)
	if len(urlResults) > 1 {
		section(w, "Combined Analysis")
		fmt.Fprintf(w, "%s:\n", utils.MakeClickableLink("https://uiperfetto.dev", "Combined Analysis"))
		fmt.Fprintf(w, "\nIncluded URLs (ordered by start time):\n")
		for i, result := range sortedResults {
			repoURL := fmt.Sprintf("https://github.com/%s/%s", result.Owner, result.Repo)
			if result.Type == "pr" {
				fmt.Fprintf(w, "  %d. %s (%s) - %s\n", i+1, utils.MakeClickableLink(result.DisplayURL, result.DisplayName), result.BranchName, utils.MakeClickableLink(repoURL, fmt.Sprintf("%s/%s", result.Owner, result.Repo)))
			} else {
				fmt.Fprintf(w, "  %d. %s - %s\n", i+1, utils.MakeClickableLink(result.DisplayURL, result.DisplayName), utils.MakeClickableLink(repoURL, fmt.Sprintf("%s/%s", result.Owner, result.Repo)))
			}
		}
		fmt.Fprintf(w, "\nCombined Pipeline Timeline:\n")
		GenerateHighLevelTimeline(w, sortedResults, globalEarliestTime, globalLatestTime)
	}

	commitAggregates := []CommitAggregate{}
	for _, result := range urlResults {
		if result.Type == "commit" {
			commitAggregates = append(commitAggregates, CommitAggregate{
				Name:                    result.DisplayName,
				URLIndex:                result.URLIndex,
				TotalRunsForCommit:      result.AllCommitRunsCount,
				TotalComputeMsForCommit: result.AllCommitRunsComputeMs,
			})
		}
	}
	if len(commitAggregates) > 0 {
		section(w, "Commit Runs (All Runs for Commit SHA)")
		for _, agg := range commitAggregates {
			computeDisplay := utils.HumanizeTime(float64(agg.TotalComputeMsForCommit) / 1000)
			fmt.Fprintf(w, "  [%d] %s: runs=%d, compute=%s\n", agg.URLIndex+1, agg.Name, agg.TotalRunsForCommit, computeDisplay)
		}
	}

	section(w, "Run Summary")
	for _, result := range urlResults {
		runsCount := result.Metrics.TotalRuns
		jobs := result.Metrics.JobTimeline
		computeMs := int64(0)
		for _, job := range jobs {
			if job.EndTime > job.StartTime {
				computeMs += job.EndTime - job.StartTime
			}
		}
		wallMs := int64(0)
		if len(jobs) > 0 {
			start := jobs[0].StartTime
			end := jobs[0].EndTime
			for _, job := range jobs {
				if job.StartTime < start {
					start = job.StartTime
				}
				if job.EndTime > end {
					end = job.EndTime
				}
			}
			wallMs = maxInt64(0, end-start)
		}
		approvals := countReviewEvents(result.ReviewEvents, "shippit") + countReviewEvents(result.ReviewEvents, "merged")
		merged := countReviewEvents(result.ReviewEvents, "merged") > 0
		fmt.Fprintf(w, "  [%d] %s: runs=%d, wall=%s, compute=%s, approvals=%d, merged=%s\n", result.URLIndex+1, result.DisplayName, runsCount, utils.HumanizeTime(float64(wallMs)/1000), utils.HumanizeTime(float64(computeMs)/1000), approvals, boolYesNo(merged))
	}

	commitResults := []analyzer.URLResult{}
	for _, result := range urlResults {
		if result.Type == "commit" {
			commitResults = append(commitResults, result)
		}
	}
	if len(commitResults) > 0 {
		section(w, "Pre-commit Runs (Before Commit Time)")
		for _, result := range commitResults {
			commitTimeMs := result.EarliestTime
			preJobs := []analyzer.TimelineJob{}
			for _, job := range result.Metrics.JobTimeline {
				if job.StartTime < commitTimeMs {
					preJobs = append(preJobs, job)
				}
			}
			if len(preJobs) == 0 {
				fmt.Fprintf(w, "  [%d] %s: none\n", result.URLIndex+1, result.DisplayName)
				continue
			}
			preCompute := int64(0)
			for _, job := range preJobs {
				end := job.EndTime
				if end > commitTimeMs {
					end = commitTimeMs
				}
				if end > job.StartTime {
					preCompute += end - job.StartTime
				}
			}
			fmt.Fprintf(w, "  [%d] %s: compute=%s across %d jobs (prior activity)\n", result.URLIndex+1, result.DisplayName, utils.HumanizeTime(float64(preCompute)/1000), len(preJobs))
		}
	}

	allJobs := append([]analyzer.CombinedTimelineJob{}, combined.JobTimeline...)
	analyzer.SortCombinedJobsByDuration(allJobs)
	slowJobs := allJobs
	if len(slowJobs) > 10 {
		slowJobs = slowJobs[:10]
	}
	if len(slowJobs) > 0 {
		section(w, "Slowest Jobs (Grouped by PR/Commit)")
		bottleneckKeys := map[string]struct{}{}
		for _, result := range sortedResults {
			for _, job := range analyzer.FindBottleneckJobs(result.Metrics.JobTimeline) {
				key := fmt.Sprintf("%s-%d-%d", job.Name, job.StartTime, job.EndTime)
				bottleneckKeys[key] = struct{}{}
			}
		}

		grouped := map[string][]analyzer.CombinedTimelineJob{}
		for _, job := range slowJobs {
			grouped[job.SourceURL] = append(grouped[job.SourceURL], job)
		}

		for _, result := range sortedResults {
			jobs := grouped[result.DisplayURL]
			if len(jobs) == 0 {
				continue
			}
			headerText := fmt.Sprintf("[%d] %s", result.URLIndex+1, result.DisplayName)
			fmt.Fprintf(w, "\n  %s:\n", utils.MakeClickableLink(result.DisplayURL, headerText))
			analyzer.SortCombinedJobsByDuration(jobs)
			for i, job := range jobs {
				duration := float64(job.EndTime-job.StartTime) / 1000
				key := fmt.Sprintf("%s-%d-%d", job.Name, job.StartTime, job.EndTime)
				bottleneck := ""
				if _, ok := bottleneckKeys[key]; ok {
					bottleneck = " 🔥"
				}
				fullText := fmt.Sprintf("%d. %s - %s%s", i+1, utils.HumanizeTime(duration), job.Name, bottleneck)
				jobLink := fullText
				if job.URL != "" {
					jobLink = utils.MakeClickableLink(job.URL, fullText)
				}
				fmt.Fprintf(w, "    %s\n", jobLink)
			}
		}
	}

	section(w, "Pipeline Timelines (OTel-native)")
	fmt.Fprintf(w, "%s:\n", utils.MakeClickableLink("https://ui.perfetto.dev", "Generic OTel Trace Waterfall"))
	RenderOTelTimeline(w, spans)

	if perfettoFile != "" {
		return perfetto.WriteTrace(w, urlResults, combined, traceEvents, globalEarliestTime, perfettoFile, openInPerfetto, spans)
	}
	return nil
}

func sortByEarliest(results []analyzer.URLResult) []analyzer.URLResult {
	sorted := append([]analyzer.URLResult{}, results...)
	sortURLResults(sorted)
	return sorted
}

func boolYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n", title)
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", len(title)))
}
