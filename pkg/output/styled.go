package output

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

// OutputStyledResults renders a lipgloss-styled terminal report.
// It writes to w (typically os.Stderr) and mirrors the sections of the TUI
// header: summary box, pending jobs, run summary, slowest jobs, and timeline.
func OutputStyledResults(w io.Writer, urlResults []analyzer.URLResult, combined analyzer.CombinedMetrics, traceEvents []analyzer.TraceEvent, globalEarliestTime, globalLatestTime int64, spans []trace.ReadOnlySpan) error {
	width := 90
	contentWidth := width - 4 // minus "│ " and " │"

	sep := labelStyle.Render(" • ")

	// ── Header box ────────────────────────────────────────────────────
	topBorder := borderStyle.Render("╭" + strings.Repeat("─", width-2) + "╮")
	botBorder := borderStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")

	buildLeftLine := func(content string) string {
		w := lipgloss.Width(content)
		pad := contentWidth - w
		if pad < 0 {
			pad = 0
		}
		return borderStyle.Render("│") + " " + content + strings.Repeat(" ", pad) + " " + borderStyle.Render("│")
	}

	// Line 1: Title
	line1 := buildLeftLine(titleStyle.Render("GitHub Actions Analyzer"))

	// Compute success rates
	successRate := float64(0)
	jobSuccessRate := float64(0)
	if combined.TotalRuns > 0 {
		// Parse from the string representation
		fmt.Sscanf(combined.SuccessRate, "%f", &successRate)
	}
	if combined.TotalJobs > 0 {
		fmt.Sscanf(combined.JobSuccessRate, "%f", &jobSuccessRate)
	}

	// Line 2: Success rates (left) + Counts (right)
	leftStyled := labelStyle.Render("Workflows: ") + colorForSuccessRate(successRate).Render(combined.SuccessRate+"%") +
		sep + labelStyle.Render("Jobs: ") + colorForSuccessRate(jobSuccessRate).Render(combined.JobSuccessRate+"%")
	rightStyled := numStyle.Render(fmt.Sprintf("%d", combined.TotalRuns)) + labelStyle.Render(" runs") +
		sep + numStyle.Render(fmt.Sprintf("%d", combined.TotalJobs)) + labelStyle.Render(" jobs") +
		sep + numStyle.Render(fmt.Sprintf("%d", combined.TotalSteps)) + labelStyle.Render(" steps")
	leftPlain := fmt.Sprintf("Workflows: %s%% • Jobs: %s%%", combined.SuccessRate, combined.JobSuccessRate)
	rightPlain := fmt.Sprintf("%d runs • %d jobs • %d steps", combined.TotalRuns, combined.TotalJobs, combined.TotalSteps)
	line2 := buildLineAligned(contentWidth, leftStyled, leftPlain, rightStyled, rightPlain)

	// Line 3: Wall time + Compute time
	wallMs, computeMs := combinedWallCompute(urlResults)
	wallTime := utils.HumanizeTime(float64(wallMs) / 1000)
	computeTime := utils.HumanizeTime(float64(computeMs) / 1000)
	leftStyled3 := labelStyle.Render("Wall: ") + numStyle.Render(wallTime) +
		sep + labelStyle.Render("Compute: ") + numStyle.Render(computeTime)
	rightStyled3 := labelStyle.Render("Concurrency: ") + numStyle.Render(fmt.Sprintf("%d", combined.MaxConcurrency))
	leftPlain3 := fmt.Sprintf("Wall: %s • Compute: %s", wallTime, computeTime)
	rightPlain3 := fmt.Sprintf("Concurrency: %d", combined.MaxConcurrency)
	line3 := buildLineAligned(contentWidth, leftStyled3, leftPlain3, rightStyled3, rightPlain3)

	fmt.Fprintln(w)
	fmt.Fprintln(w, topBorder)
	fmt.Fprintln(w, line1)
	fmt.Fprintln(w, line2)
	fmt.Fprintln(w, line3)

	// URL lines inside header box
	for _, result := range urlResults {
		urlText := result.DisplayURL
		maxW := contentWidth
		if lipgloss.Width(urlText) > maxW {
			urlText = urlText[:maxW-3] + "..."
		}
		linked := utils.MakeClickableLink(utils.ExpandGitHubURL(result.DisplayURL), urlText)
		fmt.Fprintln(w, buildLeftLine(linked))
	}
	fmt.Fprintln(w, botBorder)

	// ── Pending Jobs ──────────────────────────────────────────────────
	allPending := collectPending(urlResults)
	if len(allPending) > 0 {
		styledSection(w, "Pending Jobs")
		fmt.Fprintf(w, "  %s %d jobs still running\n",
			warningStyle.Render("WARNING:"), len(allPending))
		for i, job := range allPending {
			jobLink := utils.MakeClickableLink(job.URL, job.Name+requiredEmoji(job.IsRequired))
			fmt.Fprintf(w, "  %s  %s %s %s\n",
				dimStyle.Render(fmt.Sprintf("%d.", i+1)),
				subheaderStyle.Render(jobLink),
				dimStyle.Render("("+job.Status+")"),
				labelStyle.Render("← "+job.SourceName))
		}
	}

	// ── Run Summary ───────────────────────────────────────────────────
	sortedResults := sortByEarliest(urlResults)
	if len(urlResults) > 0 {
		styledSection(w, "Run Summary")
		// Table header
		hdr := fmt.Sprintf("  %-40s %6s %10s %10s %9s %7s",
			labelStyle.Render("URL"),
			labelStyle.Render("Runs"),
			labelStyle.Render("Wall"),
			labelStyle.Render("Compute"),
			labelStyle.Render("Approvals"),
			labelStyle.Render("Merged"))
		fmt.Fprintln(w, hdr)
		fmt.Fprintf(w, "  %s\n", dimStyle.Render(strings.Repeat("─", 86)))

		for _, result := range urlResults {
			wMs, cMs := computeTimelineDurations(result.Metrics.JobTimeline)
			approvals := countReviewEvents(result.ReviewEvents, "shippit") + countReviewEvents(result.ReviewEvents, "merged")
			merged := countReviewEvents(result.ReviewEvents, "merged") > 0
			name := result.DisplayName
			if len(name) > 38 {
				name = name[:35] + "..."
			}
			nameLinked := utils.MakeClickableLink(result.DisplayURL, name)
			mergedText := dimStyle.Render("no")
			if merged {
				mergedText = successStyle.Render("yes")
			}
			fmt.Fprintf(w, "  %-40s %s %10s %10s %9d %7s\n",
				nameLinked,
				numStyle.Render(fmt.Sprintf("%6d", result.Metrics.TotalRuns)),
				numStyle.Render(utils.HumanizeTime(float64(wMs)/1000)),
				numStyle.Render(utils.HumanizeTime(float64(cMs)/1000)),
				approvals,
				mergedText)
		}
	}

	// ── Commit Aggregates ─────────────────────────────────────────────
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
		styledSection(w, "Commit Runs (All Runs for Commit SHA)")
		for _, agg := range commitAggregates {
			computeDisplay := utils.HumanizeTime(float64(agg.TotalComputeMsForCommit) / 1000)
			fmt.Fprintf(w, "  %s %s  runs=%s  compute=%s\n",
				dimStyle.Render(fmt.Sprintf("[%d]", agg.URLIndex+1)),
				valueStyle.Render(agg.Name),
				numStyle.Render(fmt.Sprintf("%d", agg.TotalRunsForCommit)),
				numStyle.Render(computeDisplay))
		}
	}

	// ── Slowest Jobs ──────────────────────────────────────────────────
	allJobs := append([]analyzer.CombinedTimelineJob{}, combined.JobTimeline...)
	analyzer.SortCombinedJobsByDuration(allJobs)
	slowJobs := allJobs
	if len(slowJobs) > 10 {
		slowJobs = slowJobs[:10]
	}
	if len(slowJobs) > 0 {
		styledSection(w, "Slowest Jobs")
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
			fmt.Fprintf(w, "\n  %s\n", subheaderStyle.Render(utils.MakeClickableLink(result.DisplayURL, headerText)))
			analyzer.SortCombinedJobsByDuration(jobs)
			for i, job := range jobs {
				duration := float64(job.EndTime-job.StartTime) / 1000
				key := fmt.Sprintf("%s-%d-%d", job.Name, job.StartTime, job.EndTime)
				bottleneck := ""
				if _, ok := bottleneckKeys[key]; ok {
					bottleneck = " 🔥"
				}
				durationStr := utils.HumanizeTime(duration)
				jobText := fmt.Sprintf("%s %s%s%s",
					numStyle.Render(durationStr),
					valueStyle.Render(job.Name),
					bottleneck,
					requiredEmoji(job.IsRequired))
				if job.URL != "" {
					jobText = utils.MakeClickableLink(job.URL, fmt.Sprintf("%s — %s%s%s", durationStr, job.Name, bottleneck, requiredEmoji(job.IsRequired)))
				}
				fmt.Fprintf(w, "    %s  %s\n",
					dimStyle.Render(fmt.Sprintf("%d.", i+1)),
					jobText)
			}
		}
	}

	// ── Pipeline Timelines ────────────────────────────────────────────
	styledSection(w, "Pipeline Timelines")
	RenderOTelTimeline(w, spans, time.UnixMilli(globalEarliestTime), time.UnixMilli(globalLatestTime))

	return nil
}

// styledSection prints a section header with lipgloss styling.
func styledSection(w io.Writer, title string) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", titleStyle.Render(title))
	fmt.Fprintf(w, "  %s\n", dimStyle.Render(strings.Repeat("─", len(title)+2)))
}

// buildLineAligned builds a bordered line with left- and right-aligned content
// using plain-text widths for alignment.
func buildLineAligned(contentWidth int, leftStyled, leftPlain, rightStyled, rightPlain string) string {
	leftW := lipgloss.Width(leftPlain)
	rightW := lipgloss.Width(rightPlain)
	pad := contentWidth - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	return borderStyle.Render("│") + " " + leftStyled + strings.Repeat(" ", pad) + rightStyled + " " + borderStyle.Render("│")
}

// combinedWallCompute returns overall wall and compute times across all results.
func combinedWallCompute(urlResults []analyzer.URLResult) (int64, int64) {
	totalComputeMs := int64(0)
	globalStart := int64(0)
	globalEnd := int64(0)
	first := true
	for _, result := range urlResults {
		for _, job := range result.Metrics.JobTimeline {
			if job.EndTime > job.StartTime {
				totalComputeMs += job.EndTime - job.StartTime
			}
			if first || job.StartTime < globalStart {
				globalStart = job.StartTime
			}
			if first || job.EndTime > globalEnd {
				globalEnd = job.EndTime
			}
			first = false
		}
	}
	return maxInt64(0, globalEnd-globalStart), totalComputeMs
}

// RenderTimelineToBuffer renders the OTel timeline into a buffer and returns
// the content as a string. Useful for embedding in markdown code blocks.
func RenderTimelineToBuffer(spans []trace.ReadOnlySpan, globalEarliestTime, globalLatestTime int64) string {
	var buf bytes.Buffer
	RenderOTelTimeline(&buf, spans, time.UnixMilli(globalEarliestTime), time.UnixMilli(globalLatestTime))
	return buf.String()
}
