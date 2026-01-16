package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

// SpanNode represents a node in the OTel span hierarchy tree.
type SpanNode struct {
	Span     trace.ReadOnlySpan
	Children []*SpanNode
}

// BuildSpanTree constructs a hierarchy of spans based on ParentSpanID.
func BuildSpanTree(spans []trace.ReadOnlySpan) []*SpanNode {
	nodes := make(map[string]*SpanNode)
	var roots []*SpanNode

	// Create nodes for all spans
	for _, s := range spans {
		nodes[s.SpanContext().SpanID().String()] = &SpanNode{Span: s}
	}

	// Link children to parents
	for _, s := range spans {
		node := nodes[s.SpanContext().SpanID().String()]
		parentID := s.Parent().SpanID().String()
		
		if parentID == "0000000000000000" {
			roots = append(roots, node)
		} else if parent, ok := nodes[parentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			// Parent not in this batch, treat as root
			roots = append(roots, node)
		}
	}

	// Sort roots and children by start time
	sortNodes(roots)
	for _, n := range nodes {
		sortNodes(n.Children)
	}

	return roots
}

func sortNodes(nodes []*SpanNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Span.StartTime().Before(nodes[j].Span.StartTime())
	})
}

// RenderOTelTimeline renders a generic OTel span tree as a terminal waterfall.
func RenderOTelTimeline(w io.Writer, spans []trace.ReadOnlySpan) {
	if len(spans) == 0 {
		return
	}

	// Filter out internal instrumentation spans by default
	// We only want to show spans that are part of the GHA hierarchy (workflow, job, step)
	var filtered []trace.ReadOnlySpan
	for _, s := range spans {
		isGHA := false
		for _, attr := range s.Attributes() {
			if attr.Key == "type" && (attr.Value.AsString() == "workflow" || attr.Value.AsString() == "job" || attr.Value.AsString() == "step") {
				isGHA = true
				break
			}
		}
		if isGHA {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return
	}

	roots := BuildSpanTree(filtered)
	
	// Find overall time bounds
	earliest := filtered[0].StartTime()
	latest := filtered[0].EndTime()
	for _, s := range filtered {
		if s.StartTime().Before(earliest) {
			earliest = s.StartTime()
		}
		if s.EndTime().After(latest) {
			latest = s.EndTime()
		}
	}

	totalDuration := latest.Sub(earliest)
	scale := 60

	startTime := earliest.Format("15:04:05")
	endTime := latest.Format("15:04:05")
	durationStr := utils.HumanizeTime(totalDuration.Seconds())
	
	header := fmt.Sprintf("│ Start: %s   End: %s   Duration: %s", startTime, endTime, durationStr)
	padding := scale + 2 - len(header) - 1 // -1 for the trailing │
	if padding < 0 {
		padding = 0
	}

	fmt.Fprintf(w, "┌%s┐\n", strings.Repeat("─", scale+2))
	fmt.Fprintf(w, "%s%s │\n", header, strings.Repeat(" ", padding))
	fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", scale+2))

	for _, root := range roots {
		renderNode(w, root, 0, earliest, totalDuration, scale)
	}
	
	fmt.Fprintf(w, "└%s┘\n", strings.Repeat("─", scale+2))
}

func renderNode(w io.Writer, node *SpanNode, depth int, globalStart time.Time, totalDuration time.Duration, scale int) {
	s := node.Span
	start := s.StartTime().Sub(globalStart)
	duration := s.EndTime().Sub(s.StartTime())
	
	startPos := int(float64(start) / float64(totalDuration) * float64(scale))
	barLength := maxInt(1, int(float64(duration)/float64(totalDuration)*float64(scale)))
	clampedLength := minInt(barLength, scale-startPos)
	
	padding := strings.Repeat(" ", maxInt(0, startPos))
	
	// Determine color/icon from attributes
	attrs := make(map[string]string)
	for _, a := range s.Attributes() {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	
	icon := "•"
	switch attrs["type"] {
	case "workflow":
		icon = "📋"
	case "job":
		icon = "⚙️ "
	case "step":
		icon = "  ↳"
	}

	statusIcon := ""
	switch attrs["github.conclusion"] {
	case "success":
		statusIcon = "✅"
	case "failure":
		statusIcon = "❌"
	}

	barChar := "█"
	if attrs["type"] == "step" {
		barChar = "▒"
	}

	coloredBar := strings.Repeat(barChar, maxInt(1, clampedLength))
	if attrs["github.conclusion"] == "failure" {
		coloredBar = utils.RedText(coloredBar)
	} else if attrs["github.conclusion"] == "success" {
		coloredBar = utils.GreenText(coloredBar)
	} else {
		coloredBar = utils.BlueText(coloredBar)
	}

	indent := strings.Repeat("  ", depth)
	remaining := strings.Repeat(" ", maxInt(0, scale-startPos-maxInt(1, clampedLength)))

	label := s.Name()
	if url, ok := attrs["github.url"]; ok && url != "" {
		label = utils.MakeClickableLink(url, label)
	}
	displayName := fmt.Sprintf("%s%s %s", icon, statusIcon, label)

	fmt.Fprintf(w, "│%s%s%s  │ %s%s (%s)\n",
		padding, coloredBar, remaining,
		indent, displayName,
		utils.HumanizeTime(duration.Seconds()))

	for _, child := range node.Children {
		renderNode(w, child, depth+1, globalStart, totalDuration, scale)
	}
}

func GenerateHighLevelTimeline(w io.Writer, results []analyzer.URLResult, globalEarliestTime, globalLatestTime int64) {
	scale := 80
	timelineEarliest := int64(1<<63 - 1)
	timelineLatest := int64(0)

	for _, result := range results {
		if len(result.Metrics.JobTimeline) == 0 {
			continue
		}
		start := result.Metrics.JobTimeline[0].StartTime
		end := result.Metrics.JobTimeline[0].EndTime
		for _, job := range result.Metrics.JobTimeline {
			if job.StartTime < start {
				start = job.StartTime
			}
			if job.EndTime > end {
				end = job.EndTime
			}
		}
		if start < timelineEarliest {
			timelineEarliest = start
		}
		if end > timelineLatest {
			timelineLatest = end
		}
	}

	if timelineEarliest == int64(1<<63-1) {
		timelineEarliest = globalEarliestTime
	}
	if timelineLatest == 0 {
		timelineLatest = globalLatestTime
	}

	totalDuration := timelineLatest - timelineEarliest
	startLabel := fmt.Sprintf("Start: %s", time.UnixMilli(timelineEarliest).Format("3:04:05 PM"))
	endLabel := fmt.Sprintf("End: %s", time.UnixMilli(timelineLatest).Format("3:04:05 PM"))
	padding := strings.Repeat(" ", maxInt(0, scale-len(startLabel)-len(endLabel)))

	fmt.Fprintf(w, "┌%s┐\n", strings.Repeat("─", scale+2))
	fmt.Fprintf(w, "│ %s%s%s │\n", startLabel, padding, endLabel)
	fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", scale+2))

	for _, result := range results {
		if len(result.Metrics.JobTimeline) == 0 {
			continue
		}
		earliest := result.Metrics.JobTimeline[0].StartTime
		latest := result.Metrics.JobTimeline[0].EndTime
		for _, job := range result.Metrics.JobTimeline {
			if job.StartTime < earliest {
				earliest = job.StartTime
			}
			if job.EndTime > latest {
				latest = job.EndTime
			}
		}
		wallTimeSec := float64(latest-earliest) / 1000
		relativeStart := earliest - timelineEarliest
		startPos := int(float64(relativeStart) / float64(totalDuration) * float64(scale))
		maxBarLength := scale - startPos
		barLength := maxInt(1, minInt(maxBarLength, int(wallTimeSec/(float64(totalDuration)/1000)*float64(scale))))

		hasFailed := false
		hasPending := len(result.Metrics.PendingJobs) > 0
		hasSkipped := false
		for _, job := range result.Metrics.JobTimeline {
			if job.Conclusion == "failure" {
				hasFailed = true
			}
			if job.Conclusion == "skipped" || job.Conclusion == "cancelled" {
				hasSkipped = true
			}
		}

		barChars := make([]string, barLength)
		for i := range barChars {
			barChars[i] = "█"
		}
		approvalCount := 0
		for _, event := range result.ReviewEvents {
			eventTime := event.TimeMillis()
			column := int(float64(eventTime-timelineEarliest) / float64(totalDuration) * float64(scale))
			offset := column - startPos
			clamped := minInt(maxInt(offset, 0), maxInt(0, barLength-1))
			if event.Type == "merged" {
				barChars[clamped] = "◆"
			} else {
				barChars[clamped] = "▲"
				approvalCount++
			}
		}
		barString := strings.Join(barChars, "")

		fullText := fmt.Sprintf("[%d] %s (%s)", result.URLIndex+1, result.DisplayName, utils.HumanizeTime(wallTimeSec))
		var coloredBar, coloredLink string
		if hasFailed {
			coloredBar = utils.RedText(barString)
			coloredLink = utils.RedText(utils.MakeClickableLink(result.DisplayURL, fullText))
		} else if hasPending {
			coloredBar = utils.BlueText(barString)
			coloredLink = utils.BlueText(utils.MakeClickableLink(result.DisplayURL, fullText))
		} else if hasSkipped {
			coloredBar = utils.GrayText(barString)
			coloredLink = utils.GrayText(utils.MakeClickableLink(result.DisplayURL, fullText))
		} else {
			coloredBar = utils.GreenText(barString)
			coloredLink = utils.MakeClickableLink(result.DisplayURL, fullText)
		}

		paddingLeft := strings.Repeat(" ", maxInt(0, startPos))
		paddingRight := strings.Repeat(" ", maxInt(0, scale-startPos-barLength))
		suffix := ""
		if approvalCount > 0 {
			suffix = " " + utils.YellowText(fmt.Sprintf("▲ %d", approvalCount))
		}

		fmt.Fprintf(w, "│%s%s%s  │ %s%s\n", paddingLeft, coloredBar, paddingRight, coloredLink, suffix)
	}
	fmt.Fprintf(w, "└%s┘\n", strings.Repeat("─", scale+2))
}

func GenerateTimelineVisualization(w io.Writer, metrics analyzer.FinalMetrics, repoActionsURL string, urlIndex int, reviewEvents []analyzer.ReviewEvent) {
	if len(metrics.JobTimeline) == 0 {
		return
	}

	timeline := metrics.JobTimeline
	bottlenecks := analyzer.FindBottleneckJobs(timeline)
	bottleneckKeys := map[string]struct{}{}
	for _, job := range bottlenecks {
		key := fmt.Sprintf("%s-%d-%d", job.Name, job.StartTime, job.EndTime)
		bottleneckKeys[key] = struct{}{}
	}
	headerScale := 60

	earliestStart := timeline[0].StartTime
	latestEnd := timeline[0].EndTime
	for _, job := range timeline {
		if job.StartTime < earliestStart {
			earliestStart = job.StartTime
		}
		if job.EndTime > latestEnd {
			latestEnd = job.EndTime
		}
	}

	totalDuration := latestEnd - earliestStart

	fmt.Fprintf(w, "┌%s┐\n", strings.Repeat("─", headerScale+2))
	startTimeFormatted := time.UnixMilli(earliestStart).Format("3:04:05 PM")
	endTimeFormatted := time.UnixMilli(latestEnd).Format("3:04:05 PM")
	headerStart := fmt.Sprintf("Start: %s", startTimeFormatted)
	headerEnd := fmt.Sprintf("End: %s", endTimeFormatted)
	headerPadding := strings.Repeat(" ", maxInt(0, headerScale-len(headerStart)-len(headerEnd)))
	fmt.Fprintf(w, "│ %s%s%s │\n", headerStart, headerPadding, headerEnd)
	fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", headerScale+2))

	jobGroups := map[string][]analyzer.TimelineJob{}
	for _, job := range timeline {
		groupKey := utils.GetJobGroup(job.Name)
		jobGroups[groupKey] = append(jobGroups[groupKey], job)
	}

	groupNames := make([]string, 0, len(jobGroups))
	for name := range jobGroups {
		groupNames = append(groupNames, name)
	}
	sortGroupNames(groupNames, func(a, b string) bool {
		earliestA := earliestStartTime(jobGroups[a])
		earliestB := earliestStartTime(jobGroups[b])
		return earliestA < earliestB
	})

	for _, groupName := range groupNames {
		jobsInGroup := jobGroups[groupName]
		groupStart := jobsInGroup[0].StartTime
		groupEnd := jobsInGroup[0].EndTime
		for _, job := range jobsInGroup {
			if job.StartTime < groupStart {
				groupStart = job.StartTime
			}
			if job.EndTime > groupEnd {
				groupEnd = job.EndTime
			}
		}
		groupWallTime := groupEnd - groupStart
		timeDisplay := utils.HumanizeTime(float64(groupWallTime) / 1000)
		cleanGroupName := cleanLabel(groupName)
		fmt.Fprintf(w, "│%s  │ 📁 %s (%s, %d jobs)\n", strings.Repeat(" ", headerScale), cleanGroupName, timeDisplay, len(jobsInGroup))

		sortTimelineJobs(jobsInGroup, func(a, b analyzer.TimelineJob) bool {
			return a.StartTime < b.StartTime
		})
		for i, job := range jobsInGroup {
			relativeStart := job.StartTime - earliestStart
			duration := job.EndTime - job.StartTime
			durationSec := float64(duration) / 1000
			startPos := int(float64(relativeStart) / float64(totalDuration) * float64(headerScale))
			barLength := maxInt(1, int(float64(duration)/float64(totalDuration)*float64(headerScale)))
			clampedLength := minInt(barLength, headerScale-startPos)
			padding := strings.Repeat(" ", maxInt(0, startPos))

			var coloredBar string
			switch {
			case job.Conclusion == "success":
				coloredBar = utils.GreenText(strings.Repeat("█", maxInt(1, clampedLength)))
			case job.Conclusion == "failure":
				coloredBar = utils.RedText(strings.Repeat("█", maxInt(1, clampedLength)))
			case job.Status == "in_progress" || job.Status == "queued" || job.Status == "waiting":
				coloredBar = utils.BlueText(strings.Repeat("▒", maxInt(1, clampedLength)))
			case job.Conclusion == "skipped" || job.Conclusion == "cancelled":
				coloredBar = utils.GrayText(strings.Repeat("░", maxInt(1, clampedLength)))
			default:
				coloredBar = utils.GrayText(strings.Repeat("░", maxInt(1, clampedLength)))
			}

			remaining := strings.Repeat(" ", maxInt(0, headerScale-startPos-maxInt(1, clampedLength)))
			jobNameWithoutPrefix := job.Name
			if parts := strings.Split(job.Name, " / "); len(parts) > 1 {
				jobNameWithoutPrefix = strings.Join(parts[1:], " / ")
			}
			cleanJobName := cleanLabel(jobNameWithoutPrefix)
			sameNameJobs := filterJobsByName(jobsInGroup, job.Name)
			groupIndicator := ""
			if len(sameNameJobs) > 1 {
				groupIndicator = fmt.Sprintf(" [%d]", indexOfJob(sameNameJobs, job)+1)
			}
			treePrefix := "├── "
			if i == len(jobsInGroup)-1 {
				treePrefix = "└── "
			}
			jobKey := fmt.Sprintf("%s-%d-%d", job.Name, job.StartTime, job.EndTime)
			bottleneckIndicator := ""
			if _, ok := bottleneckKeys[jobKey]; ok {
				bottleneckIndicator = " 🔥"
			}
			jobNameAndTime := fmt.Sprintf("%s%s (%s)%s", cleanJobName, groupIndicator, utils.HumanizeTime(durationSec), bottleneckIndicator)
			jobLink := jobNameAndTime
			if job.URL != "" {
				jobLink = utils.MakeClickableLink(job.URL, jobNameAndTime)
			}
			statusPrefix := ""
			var displayJobText string
			switch {
			case job.Conclusion == "success":
				statusPrefix = "✅ "
				displayJobText = statusPrefix + jobLink
			case job.Conclusion == "failure":
				statusPrefix = "❌ "
				displayJobText = utils.RedText(statusPrefix + jobLink)
			case job.Status == "in_progress" || job.Status == "queued" || job.Status == "waiting":
				statusPrefix = "⏳ "
				displayJobText = utils.BlueText(statusPrefix + jobLink)
			case job.Conclusion == "skipped" || job.Conclusion == "cancelled":
				statusPrefix = "⏸️ "
				displayJobText = utils.GrayText(statusPrefix + jobLink)
			default:
				displayJobText = jobLink
			}

			fmt.Fprintf(w, "│%s%s%s  │ %s%s\n", padding, coloredBar, remaining, treePrefix, displayJobText)
		}
	}

	renderReviewMarkers(w, reviewEvents, earliestStart, latestEnd, headerScale)

	fmt.Fprintf(w, "┌%s┐\n", strings.Repeat("─", headerScale+2))
	jobCount := len(timeline)
	wallTimeSec := float64(latestEnd-earliestStart) / 1000
	footerText := fmt.Sprintf("Timeline: %s → %s • %s • %d jobs", startTimeFormatted, endTimeFormatted, utils.HumanizeTime(wallTimeSec), jobCount)
	footerLine := " " + footerText
	footerPadding := strings.Repeat(" ", maxInt(0, headerScale+2-len(footerLine)))
	fmt.Fprintf(w, "│%s%s│\n", footerLine, footerPadding)
	baseLegend := fmt.Sprintf("Legend: %s  %s  %s  %s", utils.GreenText("█ Success"), utils.RedText("█ Failed"), utils.BlueText("▒ Pending/Running"), utils.GrayText("░ Cancelled/Skipped"))
	markersLegend := ""
	if countReviewEvents(reviewEvents, "shippit") > 0 {
		markersLegend += "  " + utils.YellowText("▲ approvals")
	}
	if countReviewEvents(reviewEvents, "merged") > 0 {
		markersLegend += "  " + utils.GreenText("◆ merged")
	}
	legendContent := " " + baseLegend + markersLegend
	if len(legendContent) > headerScale+2 {
		legendContent = legendContent[:headerScale+2]
	}
	legendPadding := strings.Repeat(" ", maxInt(0, headerScale+2-len(legendContent)))
	fmt.Fprintf(w, "│%s%s│\n", legendContent, legendPadding)
	fmt.Fprintf(w, "└%s┘\n", strings.Repeat("─", headerScale+2))
}

func renderReviewMarkers(w io.Writer, events []analyzer.ReviewEvent, earliestStart, latestEnd int64, headerScale int) {
	approvalAndMerge := []analyzer.ReviewEvent{}
	for _, ev := range events {
		if ev.Type == "shippit" || ev.Type == "merged" {
			approvalAndMerge = append(approvalAndMerge, ev)
		}
	}
	if len(approvalAndMerge) == 0 {
		return
	}
	fmt.Fprintf(w, "│%s  │ 📁 Approvals & Merge (%d items)\n", strings.Repeat(" ", headerScale), len(approvalAndMerge))
	sortReviewEvents(approvalAndMerge)
	totalDuration := latestEnd - earliestStart

	markerSlots := make([]string, headerScale)
	for i := range markerSlots {
		markerSlots[i] = " "
	}
	reviewers := []string{}
	for _, ev := range approvalAndMerge {
		eventTime := ev.TimeMillis()
		relative := clampInt64(eventTime, earliestStart, latestEnd) - earliestStart
		col := int(float64(relative) / float64(totalDuration) * float64(headerScale))
		col = clampInt(col, 0, maxInt(0, headerScale-1))
		if ev.Type == "shippit" {
			markerSlots[col] = "▲"
			if ev.Reviewer != "" {
				reviewers = append(reviewers, ev.Reviewer)
			}
		}
	}
	markerLineLeft := strings.Join(markerSlots, "")
	rightParts := []string{}
	if len(reviewers) > 0 {
		rightParts = append(rightParts, utils.YellowText(fmt.Sprintf("▲ %s", reviewers[0])))
	}
	combinedRight := strings.Join(rightParts, "  ")
	maxCombinedWidth := headerScale - 4
	if len(combinedRight) > maxCombinedWidth {
		combinedRight = combinedRight[:maxCombinedWidth-3] + "..."
	}
	fmt.Fprintf(w, "│%s  │ └── %s\n", markerLineLeft, combinedRight)

	for i, ev := range approvalAndMerge {
		eventTime := ev.TimeMillis()
		relative := clampInt64(eventTime, earliestStart, latestEnd) - earliestStart
		col := int(float64(relative) / float64(totalDuration) * float64(headerScale))
		col = clampInt(col, 0, maxInt(0, headerScale-1))
		padding := strings.Repeat(" ", col)
		markerChar := "▲"
		marker := utils.YellowText(markerChar)
		if ev.Type == "merged" {
			markerChar = "◆"
			marker = utils.GreenText(markerChar)
		}
		remaining := strings.Repeat(" ", maxInt(0, headerScale-col-1))
		treePrefix := "├── "
		if i == len(approvalAndMerge)-1 {
			treePrefix = "└── "
		}
		timeStr := time.UnixMilli(eventTime).Format("3:04:05 PM")
		var rightLabel string
		if ev.Type == "merged" {
			who := "merged"
			if ev.MergedBy != "" {
				who = utils.MakeClickableLink("https://github.com/"+ev.MergedBy, ev.MergedBy)
			}
			timeLink := timeStr
			if ev.URL != "" {
				timeLink = utils.MakeClickableLink(ev.URL, timeStr)
			}
			rightLabel = utils.GreenText(fmt.Sprintf("merged by %s (%s)", who, timeLink))
		} else {
			who := "approved"
			if ev.Reviewer != "" {
				who = utils.MakeClickableLink("https://github.com/"+ev.Reviewer, ev.Reviewer)
			}
			timeLink := timeStr
			if ev.URL != "" {
				timeLink = utils.MakeClickableLink(ev.URL, timeStr)
			}
			rightLabel = utils.YellowText(fmt.Sprintf("%s (%s)", who, timeLink))
		}
		fmt.Fprintf(w, "│%s%s%s  │ %s%s\n", padding, marker, remaining, treePrefix, rightLabel)
	}
}

func cleanLabel(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(" -_/()", r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func earliestStartTime(jobs []analyzer.TimelineJob) int64 {
	if len(jobs) == 0 {
		return 0
	}
	earliest := jobs[0].StartTime
	for _, job := range jobs {
		if job.StartTime < earliest {
			earliest = job.StartTime
		}
	}
	return earliest
}

func filterJobsByName(jobs []analyzer.TimelineJob, name string) []analyzer.TimelineJob {
	filtered := []analyzer.TimelineJob{}
	for _, job := range jobs {
		if job.Name == name {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func indexOfJob(jobs []analyzer.TimelineJob, target analyzer.TimelineJob) int {
	for i, job := range jobs {
		if job.Name == target.Name && job.StartTime == target.StartTime && job.EndTime == target.EndTime {
			return i
		}
	}
	return 0
}

func countReviewEvents(events []analyzer.ReviewEvent, eventType string) int {
	count := 0
	for _, ev := range events {
		if ev.Type == eventType {
			count++
		}
	}
	return count
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampInt64(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
