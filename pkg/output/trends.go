package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
)

// formatSampleLinks renders sample URLs as clickable numbered links: " (1,2,3...)".
// The entire suffix is rendered in gray. Returns the styled string and its visible width.
func formatSampleLinks(urls []string) (string, int) {
	if len(urls) == 0 {
		return "", 0
	}
	limit := 4
	if len(urls) < limit {
		limit = len(urls)
	}
	parts := make([]string, limit)
	for i := 0; i < limit; i++ {
		parts[i] = utils.MakeClickableLink(urls[i], fmt.Sprintf("%d", i+1))
	}
	inner := strings.Join(parts, ",")
	if len(urls) > limit {
		inner += "..."
	}
	// visible width: space + ( + digits + commas + optional "..." + )
	// digits: limit, commas: limit-1, parens: 2, space: 1
	visibleWidth := 2*limit + 2
	if len(urls) > limit {
		visibleWidth += 3 // "..."
	}
	return " " + utils.GrayText("("+inner+")"), visibleWidth
}

// linkName truncates a name and appends clickable numbered sample links.
// Column padding is handled by lipgloss/table.
func linkName(name string, urls []string, maxVisible int) string {
	suffix, suffixWidth := formatSampleLinks(urls)
	nameMax := maxVisible - suffixWidth
	if nameMax < 4 {
		nameMax = 4
	}
	if len(name) > nameMax {
		name = name[:nameMax-3] + "..."
	}
	return name + suffix
}

// OutputTrends displays historical trend analysis
func OutputTrends(w io.Writer, analysis *analyzer.TrendAnalysis, format string) error {
	if format == "json" {
		return outputTrendsJSON(w, analysis)
	}

	// Default: formatted terminal output
	fmt.Fprintf(w, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(w, "📈 Historical Trend Analysis: %s/%s\n", analysis.Owner, analysis.Repo)
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 80))

	// Time range
	section(w, "Analysis Period")
	fmt.Fprintf(w, "Period: %s to %s (%d days)\n",
		analysis.TimeRange.Start.Format("Jan 02, 2006"),
		analysis.TimeRange.End.Format("Jan 02, 2006"),
		analysis.TimeRange.Days)
	fmt.Fprintf(w, "Total runs analyzed: %d\n", analysis.Summary.TotalRuns)
	if analysis.Sampling.Enabled {
		fmt.Fprintf(w, "Job details sampled: %d/%d runs (%.0f%% confidence, ±%.0f%% margin)\n",
			analysis.Sampling.SampleSize, analysis.Sampling.TotalRuns,
			analysis.Sampling.Confidence*100, analysis.Sampling.MarginOfError*100)
	}

	// Summary statistics
	section(w, "Summary Statistics")
	renderTrendSummary(w, analysis.Summary)

	// Duration trend chart
	if len(analysis.DurationTrend) > 0 {
		section(w, "Workflow Duration Trend")
		renderDurationChart(w, analysis.DurationTrend)
	}

	// Success rate trend
	if len(analysis.SuccessRateTrend) > 0 {
		section(w, "Success Rate Trend")
		renderSuccessRateChart(w, analysis.SuccessRateTrend)
	}

	// Top jobs by duration
	if len(analysis.JobTrends) > 0 {
		section(w, "Job Performance Summary")
		renderJobTrends(w, analysis.JobTrends)
	}

	// Queue time analysis
	if analysis.QueueTimeStats.AvgQueueTime > 0 {
		section(w, "Queue Time Analysis")
		renderQueueTimeStats(w, analysis.QueueTimeStats)
	}

	// Top regressions
	if len(analysis.TopRegressions) > 0 {
		section(w, "Top Performance Regressions")
		renderRegressions(w, analysis.TopRegressions)
	}

	// Top improvements
	if len(analysis.TopImprovements) > 0 {
		section(w, "Top Performance Improvements")
		renderImprovements(w, analysis.TopImprovements)
	}

	// Flaky jobs
	if len(analysis.FlakyJobs) > 0 {
		section(w, "Flaky Jobs Detected")
		renderFlakyJobs(w, analysis.FlakyJobs)
	}

	// Legend
	section(w, "Legend")
	renderLegend(w)

	return nil
}

func renderTrendSummary(w io.Writer, summary analyzer.TrendSummary) {
	fmt.Fprintf(w, "\n%-25s %20s\n", "Metric", "Value")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))

	fmt.Fprintf(w, "%-25s %20s\n", "Average Duration", utils.HumanizeTime(summary.AvgDuration))
	fmt.Fprintf(w, "%-25s %20s\n", "Median Duration", utils.HumanizeTime(summary.MedianDuration))
	fmt.Fprintf(w, "%-25s %20s\n", "95th Percentile", utils.HumanizeTime(summary.P95Duration))
	fmt.Fprintf(w, "%-25s %19.1f%%\n", "Average Success Rate", summary.AvgSuccessRate)

	// Trend direction with color
	trendDisplay := summary.TrendDirection
	percentDisplay := fmt.Sprintf("(%.1f%%)", summary.PercentChange)

	switch summary.TrendDirection {
	case "improving":
		trendDisplay = utils.GreenText("✓ Improving")
		percentDisplay = utils.GreenText(percentDisplay)
	case "degrading":
		trendDisplay = utils.RedText("⚠ Degrading")
		percentDisplay = utils.RedText(percentDisplay)
	case "stable":
		trendDisplay = utils.BlueText("→ Stable")
		percentDisplay = utils.BlueText(percentDisplay)
	}

	fmt.Fprintf(w, "%-25s %20s %s\n", "Trend Direction", trendDisplay, percentDisplay)

	if summary.TrendDescription != "" {
		fmt.Fprintf(w, "\n%s\n", summary.TrendDescription)
	}

	if summary.MostFlakyJobsCount > 0 {
		fmt.Fprintf(w, "%-25s %20d\n", "Flaky Jobs Detected", summary.MostFlakyJobsCount)
	}
}

func renderDurationChart(w io.Writer, points []analyzer.DataPoint) {
	if len(points) == 0 {
		return
	}

	// Render ASCII chart
	fmt.Fprintf(w, "\n")
	chart := generateASCIIChart(points, 40, 10, "seconds")
	fmt.Fprint(w, chart)
}

func renderSuccessRateChart(w io.Writer, points []analyzer.DataPoint) {
	if len(points) == 0 {
		return
	}

	// Render ASCII chart
	fmt.Fprintf(w, "\n")
	chart := generateASCIIChart(points, 40, 10, "percent")
	fmt.Fprint(w, chart)
}

func renderJobTrends(w io.Writer, trends []analyzer.JobTrend) {
	// Show top 10 jobs
	limit := 10
	if len(trends) < limit {
		limit = len(trends)
	}

	fmt.Fprintf(w, "\nTop %d Jobs by Average Duration:\n\n", limit)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true)
			}
			if col == 0 {
				return lipgloss.NewStyle()
			}
			if col == 4 {
				return lipgloss.NewStyle().Align(lipgloss.Center)
			}
			return lipgloss.NewStyle().Align(lipgloss.Right)
		}).
		Headers("Job Name", "Avg Duration", "Median", "Success Rate", "Trend")

	for i := 0; i < limit; i++ {
		job := trends[i]

		trendIcon := "→"
		trendColor := utils.BlueText
		switch job.TrendDirection {
		case "improving":
			trendIcon = "✓"
			trendColor = utils.GreenText
		case "degrading":
			trendIcon = "⚠"
			trendColor = utils.RedText
		}

		t.Row(
			linkName(job.Name, job.URLs, 48),
			utils.HumanizeTime(job.AvgDuration),
			utils.HumanizeTime(job.MedianDuration),
			fmt.Sprintf("%.1f%%", job.SuccessRate),
			trendColor(trendIcon),
		)
	}

	fmt.Fprintln(w, t)

	if len(trends) > limit {
		fmt.Fprintf(w, "\n... and %d more jobs\n", len(trends)-limit)
	}
}

func renderFlakyJobs(w io.Writer, flakyJobs []analyzer.FlakyJob) {
	fmt.Fprintf(w, "\n%s Found %d flaky jobs (>10%% failure rate):\n\n",
		utils.YellowText("⚠️"),
		len(flakyJobs))

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true)
			}
			if col == 0 {
				return lipgloss.NewStyle()
			}
			return lipgloss.NewStyle().Align(lipgloss.Right)
		}).
		Headers("Job Name", "Total Runs", "Failures", "Flake Rate", "Recent (10)")

	for _, job := range flakyJobs {
		flakeRateColor := utils.YellowText
		if job.FlakeRate > 30 {
			flakeRateColor = utils.RedText
		}

		t.Row(
			linkName(job.Name, job.URLs, 48),
			fmt.Sprintf("%d", job.TotalRuns),
			fmt.Sprintf("%d", job.FailureCount),
			flakeRateColor(fmt.Sprintf("%.1f%%", job.FlakeRate)),
			fmt.Sprintf("%d", job.RecentFailures),
		)
	}

	fmt.Fprintln(w, t)

	fmt.Fprintf(w, "\n%s Recommendations:\n", utils.BlueText("💡"))
	fmt.Fprintf(w, "   • Investigate flaky jobs for race conditions or timing issues\n")
	fmt.Fprintf(w, "   • Consider adding retries or improving test stability\n")
	fmt.Fprintf(w, "   • Review recent failures for common patterns\n")
}

// generateASCIIChart creates a simple ASCII line chart
func generateASCIIChart(points []analyzer.DataPoint, width, height int, valueType string) string {
	if len(points) == 0 {
		return ""
	}

	// Find min/max values
	minVal := points[0].Value
	maxVal := points[0].Value
	for _, p := range points {
		if p.Value < minVal {
			minVal = p.Value
		}
		if p.Value > maxVal {
			maxVal = p.Value
		}
	}

	// Add padding to range
	valueRange := maxVal - minVal
	if valueRange == 0 {
		valueRange = 1
	}
	padding := valueRange * 0.1
	minVal -= padding
	maxVal += padding

	var sb strings.Builder

	// Build chart from top to bottom
	for row := height - 1; row >= 0; row-- {
		// Calculate value threshold for this row
		threshold := minVal + (float64(row)/float64(height-1))*(maxVal-minVal)

		// Y-axis label
		label := formatChartValue(threshold, valueType)
		sb.WriteString(fmt.Sprintf("%8s │", label))

		// Plot points
		for col := 0; col < width; col++ {
			// Map column to data point
			pointIdx := int(math.Round(float64(col) / float64(width-1) * float64(len(points)-1)))
			if pointIdx >= len(points) {
				pointIdx = len(points) - 1
			}

			value := points[pointIdx].Value

			// Determine if we should plot here
			nextThreshold := minVal + (float64(row+1)/float64(height-1))*(maxVal-minVal)
			if value >= threshold && value < nextThreshold {
				sb.WriteString("●")
			} else if value > threshold {
				sb.WriteString("│")
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\n")
	}

	// X-axis
	sb.WriteString(fmt.Sprintf("%8s └%s\n", "", strings.Repeat("─", width)))

	// Time labels
	if len(points) >= 2 {
		startDate := points[0].Timestamp.Format("Jan 02")
		endDate := points[len(points)-1].Timestamp.Format("Jan 02")
		sb.WriteString(fmt.Sprintf("%10s%s%*s\n", startDate, "", width-len(startDate), endDate))
	}

	return sb.String()
}

func formatChartValue(value float64, valueType string) string {
	switch valueType {
	case "percent":
		return fmt.Sprintf("%.0f%%", value)
	case "seconds":
		if value < 60 {
			return fmt.Sprintf("%.0fs", value)
		} else if value < 3600 {
			return fmt.Sprintf("%.0fm", value/60)
		}
		return fmt.Sprintf("%.1fh", value/3600)
	default:
		return fmt.Sprintf("%.1f", value)
	}
}

// outputTrendsJSON outputs trends as JSON
func outputTrendsJSON(w io.Writer, analysis *analyzer.TrendAnalysis) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(analysis)
}

func renderQueueTimeStats(w io.Writer, stats analyzer.QueueTimeStats) {
	fmt.Fprintf(w, "\n%-25s %20s\n", "Metric", "Value")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))

	fmt.Fprintf(w, "%-25s %20s\n", "Average Queue Time", utils.HumanizeTime(stats.AvgQueueTime))
	fmt.Fprintf(w, "%-25s %20s\n", "Median Queue Time", utils.HumanizeTime(stats.MedianQueueTime))
	fmt.Fprintf(w, "%-25s %20s\n", "95th Percentile Queue", utils.HumanizeTime(stats.P95QueueTime))
	fmt.Fprintf(w, "%-25s %20s\n", "Average Run Time", utils.HumanizeTime(stats.AvgRunTime))
	fmt.Fprintf(w, "%-25s %20s\n", "Median Run Time", utils.HumanizeTime(stats.MedianRunTime))
	fmt.Fprintf(w, "%-25s %19.1f%%\n", "Queue Time Ratio", stats.QueueTimeRatio)

	// Provide context on queue time
	if stats.QueueTimeRatio > 50 {
		fmt.Fprintf(w, "\n%s Jobs spend more time waiting than running. Consider:\n", utils.YellowText("⚠️"))
		fmt.Fprintf(w, "   • Adding more runners\n")
		fmt.Fprintf(w, "   • Using self-hosted runners for faster startup\n")
		fmt.Fprintf(w, "   • Optimizing job concurrency limits\n")
	} else if stats.QueueTimeRatio > 25 {
		fmt.Fprintf(w, "\n%s Queue time is moderate. Monitor for spikes during peak hours.\n", utils.BlueText("💡"))
	} else {
		fmt.Fprintf(w, "\n%s Queue time is healthy. Jobs start quickly.\n", utils.GreenText("✓"))
	}
}

func renderRegressions(w io.Writer, regressions []analyzer.JobRegression) {
	fmt.Fprintf(w, "\n%s Jobs that got significantly slower (>10%% increase):\n\n", utils.RedText("⚠️"))

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true)
			}
			if col == 0 {
				return lipgloss.NewStyle()
			}
			return lipgloss.NewStyle().Align(lipgloss.Right)
		}).
		Headers("Job Name", "Was", "Now", "Change")

	for _, reg := range regressions {
		t.Row(
			linkName(reg.Name, reg.URLs, 58),
			utils.HumanizeTime(reg.OldAvgDuration),
			utils.HumanizeTime(reg.NewAvgDuration),
			failureStyle.Render(fmt.Sprintf("+%.1f%%", reg.PercentIncrease)),
		)
	}

	fmt.Fprintln(w, t)

	fmt.Fprintf(w, "\n%s Investigate these jobs for:\n", utils.BlueText("💡"))
	fmt.Fprintf(w, "   • Recent code changes that may have added overhead\n")
	fmt.Fprintf(w, "   • Missing or invalid caches\n")
	fmt.Fprintf(w, "   • Resource contention or runner performance issues\n")
	fmt.Fprintf(w, "   • Dependencies that need updating or optimization\n")
}

func renderImprovements(w io.Writer, improvements []analyzer.JobImprovement) {
	fmt.Fprintf(w, "\n%s Jobs that got significantly faster (>10%% decrease):\n\n", utils.GreenText("✓"))

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true)
			}
			if col == 0 {
				return lipgloss.NewStyle()
			}
			return lipgloss.NewStyle().Align(lipgloss.Right)
		}).
		Headers("Job Name", "Was", "Now", "Change")

	for _, imp := range improvements {
		t.Row(
			linkName(imp.Name, imp.URLs, 58),
			utils.HumanizeTime(imp.OldAvgDuration),
			utils.HumanizeTime(imp.NewAvgDuration),
			successStyle.Render(fmt.Sprintf("-%.1f%%", imp.PercentDecrease)),
		)
	}

	fmt.Fprintln(w, t)
}

func renderLegend(w io.Writer) {
	fmt.Fprintf(w, "\n%s Trend Indicators:\n", utils.BlueText("ℹ️"))
	fmt.Fprintf(w, "   %s  Job got faster (>5%% improvement)\n", utils.GreenText("✓"))
	fmt.Fprintf(w, "   %s  Job got slower (>5%% regression)\n", utils.RedText("⚠"))
	fmt.Fprintf(w, "   %s  Job performance is stable (<5%% change)\n", utils.BlueText("→"))
}
