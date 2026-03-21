package results

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/stefanpenner/otel-analyzer/pkg/utils"
)

const (
	defaultTreeWidth = 55
	minTreeWidth     = 25
	maxTreeWidth     = 120
	treeWidthStep    = 5
	horizontalPad    = 2 // left/right padding for main view
)

// highlightMatch splits name into before/match/after and styles the match portion
// with charStyle, and the rest with rowStyle. The match is case-insensitive.
func highlightMatch(name, query string, charStyle, rowStyle lipgloss.Style) string {
	lower := strings.ToLower(name)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		return rowStyle.Render(name)
	}
	before := name[:idx]
	match := name[idx : idx+len(query)]
	after := name[idx+len(query):]

	var result string
	if before != "" {
		result += rowStyle.Render(before)
	}
	result += charStyle.Render(match)
	if after != "" {
		result += rowStyle.Render(after)
	}
	return result
}

// hyperlink wraps text in OSC 8 terminal hyperlink escape sequence.
// This makes the text clickable in supporting terminals (iTerm2, Kitty, WezTerm, etc.)
// The id parameter ensures terminals treat each link independently.
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	// OSC 8 format with id: \x1b]8;id=ID;URL\x07TEXT\x1b]8;;\x07
	// Using URL as ID ensures same URLs are grouped, different URLs are independent
	return fmt.Sprintf("\x1b]8;id=%s;%s\x07%s\x1b]8;;\x07", url, url, text)
}

// colorForRate returns a style based on the success rate value
func colorForRate(rate float64) lipgloss.Style {
	switch {
	case rate >= 100:
		return lipgloss.NewStyle().Foreground(ColorGreen)
	case rate >= 80:
		return lipgloss.NewStyle().Foreground(ColorOffWhite) // normal
	case rate >= 50:
		return lipgloss.NewStyle().Foreground(ColorYellow)
	default:
		return lipgloss.NewStyle().Foreground(ColorMagenta)
	}
}

// padRight pads a string to the given width (using plain text width calculation)
func padRight(styled, plain string, width int) string {
	plainWidth := lipgloss.Width(plain)
	if plainWidth >= width {
		return styled
	}
	return styled + strings.Repeat(" ", width-plainWidth)
}

// renderHeader renders the title bar with statistics
func (m Model) renderHeader() string {
	width := m.width
	if width < 40 {
		width = 40
	}
	totalWidth := width - horizontalPad*2
	if totalWidth < 1 {
		totalWidth = 80
	}
	contentWidth := totalWidth - 4 // minus "│ " and " │"
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Styles
	numStyle := lipgloss.NewStyle().Foreground(ColorBlue)
	sep := HeaderCountStyle.Render(" • ")

	// Build borders
	topBorder := BorderStyle.Render("╭" + strings.Repeat("─", max(0, totalWidth-2)) + "╮")

	// Helper to build a line with left content and optional right content
	buildLine := func(left, leftPlain, right, rightPlain string) string {
		leftWidth := lipgloss.Width(leftPlain)
		rightWidth := lipgloss.Width(rightPlain)
		middlePad := contentWidth - leftWidth - rightWidth
		if middlePad < 1 {
			middlePad = 1
		}
		return BorderStyle.Render("│") + " " + left + strings.Repeat(" ", middlePad) + right + " " + BorderStyle.Render("│")
	}

	// Helper to build a simple left-aligned line
	buildLeftLine := func(content, plain string) string {
		w := lipgloss.Width(plain)
		pad := contentWidth - w
		if pad < 0 {
			pad = 0
		}
		return BorderStyle.Render("│") + " " + content + strings.Repeat(" ", pad) + " " + BorderStyle.Render("│")
	}

	// Line 1: Title
	line1 := buildLeftLine(HeaderStyle.Render("Trace Analyzer"), "Trace Analyzer")

	// Calculate rates
	successRate := float64(0)
	if m.displayedSummary.TotalRuns > 0 {
		successRate = float64(m.displayedSummary.SuccessfulRuns) / float64(m.displayedSummary.TotalRuns) * 100
	}
	jobSuccessRate := float64(0)
	if m.displayedSummary.TotalJobs > 0 {
		jobSuccessRate = float64(m.displayedSummary.TotalJobs-m.displayedSummary.FailedJobs) / float64(m.displayedSummary.TotalJobs) * 100
	}

	// Line 2: Success rates (left) + Counts (right)
	// Left side: "Workflows: 100% • Jobs: 100%"
	leftStyled := HeaderCountStyle.Render("Traces: ") + colorForRate(successRate).Render(fmt.Sprintf("%.0f%%", successRate)) +
		sep + HeaderCountStyle.Render("Spans: ") + colorForRate(jobSuccessRate).Render(fmt.Sprintf("%.0f%%", jobSuccessRate))
	leftPlain := fmt.Sprintf("Traces: %.0f%% • Spans: %.0f%%", successRate, jobSuccessRate)

	// Right side: "1 traces • 3 spans • 21 leaves"
	rightStyled := numStyle.Render(fmt.Sprintf("%d", m.displayedSummary.TotalRuns)) + HeaderCountStyle.Render(" traces") +
		sep + numStyle.Render(fmt.Sprintf("%d", m.displayedSummary.TotalJobs)) + HeaderCountStyle.Render(" spans") +
		sep + numStyle.Render(fmt.Sprintf("%d", m.displayedStepCount)) + HeaderCountStyle.Render(" leaves")
	rightPlain := fmt.Sprintf("%d traces • %d spans • %d leaves", m.displayedSummary.TotalRuns, m.displayedSummary.TotalJobs, m.displayedStepCount)

	line2 := buildLine(leftStyled, leftPlain, rightStyled, rightPlain)

	// Line 3: Times (left) + Concurrency (right)
	wallTime := utils.HumanizeTime(float64(m.displayedWallTimeMs) / 1000)
	computeTime := utils.HumanizeTime(float64(m.displayedComputeMs) / 1000)

	leftStyled3 := HeaderCountStyle.Render("Wall: ") + numStyle.Render(wallTime) +
		sep + HeaderCountStyle.Render("Compute: ") + numStyle.Render(computeTime)
	leftPlain3 := fmt.Sprintf("Wall: %s • Compute: %s", wallTime, computeTime)

	// Add effective time when logical end marker is set
	if m.logicalEndID != "" {
		effectiveSecs := m.logicalEndTime.Sub(m.chartStart).Seconds()
		if effectiveSecs < 0 {
			effectiveSecs = 0
		}
		effectiveTime := utils.HumanizeTime(effectiveSecs)
		leftStyled3 += sep + HeaderCountStyle.Render("Effective: ") + LogicalEndBadgeStyle.Render(effectiveTime)
		leftPlain3 += fmt.Sprintf(" • Effective: %s", effectiveTime)
	}

	rightStyled3 := HeaderCountStyle.Render("Concurrency: ") + numStyle.Render(fmt.Sprintf("%d", m.displayedSummary.MaxConcurrency))
	rightPlain3 := fmt.Sprintf("Concurrency: %d", m.displayedSummary.MaxConcurrency)

	line3 := buildLine(leftStyled3, leftPlain3, rightStyled3, rightPlain3)

	// Line 4: Queue + Retry + Billable (conditional)
	line4 := ""
	{
		var parts []string
		var partsPlain []string

		// Queue time
		if m.displayedSummary.QueueCount > 0 {
			avgQ := utils.HumanizeTime(m.displayedSummary.AvgQueueTimeMs / 1000)
			maxQ := utils.HumanizeTime(m.displayedSummary.MaxQueueTimeMs / 1000)
			parts = append(parts, HeaderCountStyle.Render("Queue: avg ")+numStyle.Render(avgQ)+HeaderCountStyle.Render(" / max ")+numStyle.Render(maxQ))
			partsPlain = append(partsPlain, fmt.Sprintf("Queue: avg %s / max %s", avgQ, maxQ))
		}

		// Retry rate
		if m.displayedSummary.RetriedRuns > 0 && m.displayedSummary.TotalRuns > 0 {
			retryPct := fmt.Sprintf("%.0f%%", float64(m.displayedSummary.RetriedRuns)/float64(m.displayedSummary.TotalRuns)*100)
			parts = append(parts, HeaderCountStyle.Render("Retries: ")+numStyle.Render(retryPct))
			partsPlain = append(partsPlain, fmt.Sprintf("Retries: %s", retryPct))
		}

		// Billable total
		var totalBillableMs int64
		for _, ms := range m.displayedSummary.BillableMs {
			totalBillableMs += ms
		}
		if totalBillableMs > 0 {
			billStr := utils.HumanizeTime(float64(totalBillableMs) / 1000)
			parts = append(parts, HeaderCountStyle.Render("Billable: ")+numStyle.Render(billStr))
			partsPlain = append(partsPlain, fmt.Sprintf("Billable: %s", billStr))
		}

		if len(parts) > 0 {
			styled4 := strings.Join(parts, sep)
			plain4 := strings.Join(partsPlain, " • ")
			line4 = "\n" + buildLeftLine(styled4, plain4)
		}
	}

	// URL lines
	lineURL := ""
	for _, inputURL := range m.inputURLs {
		urlText := inputURL
		maxURLWidth := contentWidth
		if lipgloss.Width(urlText) > maxURLWidth {
			urlText = urlText[:maxURLWidth-3] + "..."
		}
		linkedURL := hyperlink(utils.ExpandGitHubURL(inputURL), urlText)
		lineURL += "\n" + buildLeftLine(linkedURL, urlText)
	}

	return topBorder + "\n" + line1 + "\n" + line2 + "\n" + line3 + line4 + lineURL
}

// renderTimeAxis renders the time axis row that sits above the timeline
// It shows start time aligned with left edge, duration centered, end time at right edge
func (m Model) renderTimeAxis() string {
	width := m.width
	if width < 40 {
		width = 40
	}
	totalWidth := width - horizontalPad*2
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Match the structure of item rows: │ tree │ timeline │
	treeW := m.treeWidth
	availableW := totalWidth - 3 // 3 border chars
	timelineW := availableW - treeW
	if timelineW < 10 {
		timelineW = 10
	}

	// Tree part is empty (just padding to align with timeline)
	treePart := strings.Repeat(" ", treeW)

	// Build time axis for the timeline area
	if m.chartStart.IsZero() || m.chartEnd.IsZero() {
		// No time data, just return empty row
		return BorderStyle.Render("│") + treePart + SeparatorStyle.Render("│") + strings.Repeat(" ", timelineW) + BorderStyle.Render("│")
	}

	startTime := m.chartStart.Format("15:04:05")
	endTime := m.chartEnd.Format("15:04:05")
	durationSecs := m.chartEnd.Sub(m.chartStart).Seconds()
	if durationSecs < 0 {
		durationSecs = 0
	}
	duration := utils.HumanizeTime(durationSecs)

	// Style for numeric values
	numStyle := lipgloss.NewStyle().Foreground(ColorBlue)

	startW := lipgloss.Width(startTime)
	durW := lipgloss.Width(duration)
	endW := lipgloss.Width(endTime)

	// Calculate gaps: start...duration...end to fill timelineW
	// We want duration roughly centered
	totalTextW := startW + durW + endW
	remainingSpace := timelineW - totalTextW
	if remainingSpace < 2 {
		remainingSpace = 2
	}

	// Put duration in center, start at left, end at right
	leftGap := (timelineW - durW) / 2 - startW
	if leftGap < 1 {
		leftGap = 1
	}
	rightGap := timelineW - startW - leftGap - durW - endW
	if rightGap < 1 {
		rightGap = 1
	}

	// Build the timeline axis content as a rune buffer so we can overlay the ▼ marker
	axisRunes := make([]rune, timelineW)
	// Fill: start text, left gap dashes, duration text, right gap dashes, end text
	pos := 0
	for _, r := range startTime {
		if pos < timelineW {
			axisRunes[pos] = r
			pos++
		}
	}
	for i := 0; i < leftGap && pos < timelineW; i++ {
		axisRunes[pos] = '─'
		pos++
	}
	for _, r := range duration {
		if pos < timelineW {
			axisRunes[pos] = r
			pos++
		}
	}
	for i := 0; i < rightGap && pos < timelineW; i++ {
		axisRunes[pos] = '─'
		pos++
	}
	for _, r := range endTime {
		if pos < timelineW {
			axisRunes[pos] = r
			pos++
		}
	}
	// Fill remaining with spaces
	for pos < timelineW {
		axisRunes[pos] = ' '
		pos++
	}

	// Overlay ▼ marker at logical end position if set
	logicalEndPos := -1
	if m.logicalEndID != "" && !m.logicalEndTime.IsZero() && !m.chartStart.IsZero() && !m.chartEnd.IsZero() {
		chartDuration := m.chartEnd.Sub(m.chartStart)
		if chartDuration > 0 {
			endOffset := m.logicalEndTime.Sub(m.chartStart)
			logicalEndPos = int(float64(endOffset) / float64(chartDuration) * float64(timelineW))
			if logicalEndPos >= timelineW {
				logicalEndPos = timelineW - 1
			}
			if logicalEndPos < 0 {
				logicalEndPos = 0
			}
			axisRunes[logicalEndPos] = '▼'
		}
	}

	// Build styled output character by character
	var timelineContent strings.Builder
	for i, r := range axisRunes {
		ch := string(r)
		if i == logicalEndPos {
			timelineContent.WriteString(LogicalEndBadgeStyle.Render(ch))
		} else if r == '─' {
			timelineContent.WriteString(ch)
		} else {
			timelineContent.WriteString(numStyle.Render(ch))
		}
	}

	return BorderStyle.Render("│") + treePart + SeparatorStyle.Render("│") + timelineContent.String() + BorderStyle.Render("│")
}

// renderItem renders a single tree item with timeline bar
func (m Model) renderItem(item TreeItem, isSelected bool) string {
	width := m.width
	if width < 40 {
		width = 40
	}
	totalWidth := width - horizontalPad*2 // account for left/right padding
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Calculate widths
	// Line structure: │ + treePart + │ + timelineBar + │ = 3 border chars
	availableWidth := totalWidth - 3 // 3 border characters
	treeW := m.treeWidth
	timelineW := availableWidth - treeW
	if timelineW < 10 {
		timelineW = 10
	}

	// Build indent
	// Steps align under their parent job icon, so use parent's depth
	indentDepth := item.Depth
	if item.ItemType == ItemTypeLeaf && indentDepth > 0 {
		indentDepth = indentDepth - 1
	}
	indent := strings.Repeat("  ", indentDepth)
	indentWidth := indentDepth * 2

	// Expand indicator
	expandIndicator := " "
	if item.HasChildren {
		if m.expandedState[item.ID] {
			expandIndicator = "▼"
		} else {
			expandIndicator = "▶"
		}
	}
	expandWidth := 1

	// Get icon based on item type
	icon := getItemIcon(item)
	iconWidth := GetCharWidth(icon)

	// Get status indicator
	statusIcon := getStatusIcon(item)
	statusWidth := GetCharWidth(statusIcon)

	// Get badges
	badges := getBadges(item)
	badgesWidth := getBadgesWidth(badges)

	// Check logical end state (needed early for badge width calculation)
	isLogicalEnd := item.ID == m.logicalEndID
	isAfterEnd := m.isAfterLogicalEnd(item)

	// Add [end] badge for logical end marker
	endBadgeWidth := 0
	if isLogicalEnd {
		endBadgeWidth = 6 // len(" [end]")
	}

	// Build the name part
	name := item.DisplayName
	if item.Hints.User != "" && item.Hints.IsMarker {
		name = fmt.Sprintf("%s by %s", name, item.Hints.User)
	}

	// Build duration string separately (styled in gray)
	durationStr := ""
	durationWidth := 0
	if !item.StartTime.IsZero() && !item.EndTime.IsZero() {
		duration := item.EndTime.Sub(item.StartTime).Seconds()
		if duration < 0 {
			duration = 0
		}
		durationStr = fmt.Sprintf(" (%s)", utils.HumanizeTime(duration))
		durationWidth = lipgloss.Width(durationStr)
	}

	// Calculate available space for name
	// Format: indent + expand + space + icon + space + name + duration + badges + endBadge + space + status
	usedWidth := indentWidth + expandWidth + 1 + iconWidth + 1 + durationWidth + badgesWidth + endBadgeWidth + 1 + statusWidth
	maxNameWidth := treeW - usedWidth
	if maxNameWidth < 5 {
		maxNameWidth = 5
	}

	// Truncate name if needed
	nameWidth := lipgloss.Width(name)
	if nameWidth > maxNameWidth {
		// Truncate to fit
		truncated := ""
		w := 0
		for _, r := range name {
			rw := lipgloss.Width(string(r))
			if w+rw+3 > maxNameWidth { // +3 for "..."
				break
			}
			truncated += string(r)
			w += rw
		}
		name = truncated + "..."
		nameWidth = lipgloss.Width(name)
	}

	// Calculate tree part width from known component widths (avoids issues with escape sequences)
	// Format: indent + expand + space + icon + space + name + duration + badges + endBadge + space + status
	treePartWidth := indentWidth + expandWidth + 1 + iconWidth + 1 + nameWidth + durationWidth + badgesWidth + endBadgeWidth + 1 + statusWidth

	// Pad tree part to fixed width
	treePadding := treeW - treePartWidth
	if treePadding < 0 {
		treePadding = 0
	}

	// Check if item has collapsed children (for sparkline markers)
	hasCollapsedChildren := item.HasChildren && !m.expandedState[item.ID]

	// Check if item is hidden from chart
	isHidden := m.hiddenState[item.ID]

	// Check if item is a search match for two-tone highlighting
	isSearchMatch := m.searchMatchIDs[item.ID]

	// Wrap name in hyperlink if URL is available (must be done after width calculation)
	// Apply two-tone search match highlighting: row gets subtle bg, matching chars get stronger style
	displayName := name
	if isSearchMatch && m.searchQuery != "" {
		if isSelected {
			displayName = highlightMatch(name, m.searchQuery, SearchCharSelectedStyle, SelectedStyle)
		} else {
			displayName = highlightMatch(name, m.searchQuery, SearchCharStyle, SearchRowStyle)
		}
	}
	displayName = hyperlink(item.Hints.URL, displayName)

	// Build styled [end] badge
	styledEndBadge := ""
	if isLogicalEnd {
		styledEndBadge = LogicalEndBadgeStyle.Render(" [end]")
	}

	// Build tree part content
	// When selected, every segment must carry the selection background because
	// each lipgloss Render() ends with an ANSI reset that kills the outer background.
	var treePart string
	if isSelected {
		sel := SelectedStyle
		if isHidden || isAfterEnd {
			sel = HiddenSelectedStyle
		}
		selDur := FooterStyle.Background(ColorSelectionBg)
		prefix := fmt.Sprintf("%s%s %s %s", indent, expandIndicator, icon, displayName)
		treePart = sel.Render(prefix)
		if durationStr != "" {
			treePart += selDur.Render(durationStr)
		}
		treePart += sel.Render(badges)
		if isLogicalEnd {
			treePart += LogicalEndBadgeStyle.Background(ColorSelectionBg).Render(" [end]")
		}
		treePart += sel.Render(" ") + getStyledStatusIconWithBg(item, ColorSelectionBg)
	} else if isSearchMatch {
		// Search match row: subtle purple-tinted background
		row := SearchRowStyle
		rowDur := FooterStyle.Background(ColorSearchRowBg)
		prefix := fmt.Sprintf("%s%s %s %s", indent, expandIndicator, icon, displayName)
		treePart = row.Render(prefix)
		if durationStr != "" {
			treePart += rowDur.Render(durationStr)
		}
		treePart += row.Render(badges)
		if isLogicalEnd {
			treePart += LogicalEndBadgeStyle.Background(ColorSearchRowBg).Render(" [end]")
		}
		treePart += row.Render(" ") + getStyledStatusIconWithBg(item, ColorSearchRowBg)
	} else if isAfterEnd {
		// After logical end: render in gray (dimmed)
		styledDuration := ""
		if durationStr != "" {
			styledDuration = HiddenStyle.Render(durationStr)
		}
		treePart = HiddenStyle.Render(fmt.Sprintf("%s%s %s %s", indent, expandIndicator, icon, displayName)) +
			styledDuration + HiddenStyle.Render(badges+" ") + HiddenStyle.Render(getStatusIcon(item))
	} else {
		styledDuration := ""
		if durationStr != "" {
			styledDuration = FooterStyle.Render(durationStr)
		}
		styledStatusIcon := getStyledStatusIcon(item)
		treePart = fmt.Sprintf("%s%s %s %s%s%s", indent, expandIndicator, icon, displayName, styledDuration, badges) +
			styledEndBadge + fmt.Sprintf(" %s", styledStatusIcon)
	}

	// Render timeline bar (empty if hidden, dimmed colors if selected, full colors otherwise)
	// For normal items, URL is passed so bar characters are clickable.
	// For selected/hidden items, URL is omitted since we apply row-level hyperlink at the end.
	// For collapsed items with children, overlay dimmed child markers as a sparkline summary.
	var timelineBar string
	if isHidden && isSelected {
		// Hidden + selected: empty timeline with selection background
		timelineBar = SelectedBgStyle.Render(strings.Repeat(" ", timelineW))
	} else if isHidden {
		timelineBar = strings.Repeat(" ", timelineW)
	} else if isAfterEnd && isSelected {
		// After logical end + selected: dimmed bar with selection background
		timelineBar = RenderTimelineBarDimmedSelected(item, m.chartStart, m.chartEnd, timelineW)
	} else if isAfterEnd {
		// After logical end: dimmed gray bar
		timelineBar = RenderTimelineBarDimmed(item, m.chartStart, m.chartEnd, timelineW)
	} else if isSelected && hasCollapsedChildren {
		timelineBar = RenderTimelineBarWithChildrenSelected(item, m.chartStart, m.chartEnd, timelineW, "")
	} else if isSelected {
		// Render with dimmed colors and selection background
		timelineBar = RenderTimelineBarSelected(item, m.chartStart, m.chartEnd, timelineW, "")
	} else if isSearchMatch && hasCollapsedChildren {
		timelineBar = renderTimelineBarWithChildrenBg(item, m.chartStart, m.chartEnd, timelineW, item.Hints.URL, SearchRowBgStyle)
	} else if isSearchMatch {
		// Search match: normal bar colors but with subtle row background on empty space
		timelineBar = renderTimelineBarWithBg(item, m.chartStart, m.chartEnd, timelineW, item.Hints.URL, SearchRowBgStyle)
	} else if hasCollapsedChildren {
		timelineBar = RenderTimelineBarWithChildren(item, m.chartStart, m.chartEnd, timelineW, item.Hints.URL)
	} else {
		// Normal: full colors, pass URL so bar is clickable
		timelineBar = RenderTimelineBar(item, m.chartStart, m.chartEnd, timelineW, item.Hints.URL)
	}

	// Overlay logical end vertical line on the timeline bar
	endCol := m.logicalEndCol(timelineW)
	if endCol >= 0 {
		timelineBar = overlayLogicalEndLine(timelineBar, endCol, timelineW, isSelected)
	}

	// Combine with styled borders
	midSep := SeparatorStyle.Render("│")

	// Padding is rendered separately so that inner ANSI resets (from styled
	// status icons, durations, etc.) don't kill the selection background.
	if isSelected && (isHidden || isAfterEnd) {
		pad := SelectedBgStyle.Render(strings.Repeat(" ", treePadding))
		return BorderStyle.Render("│") + treePart + pad + midSep + timelineBar + BorderStyle.Render("│")
	} else if isSelected {
		pad := SelectedBgStyle.Render(strings.Repeat(" ", treePadding))
		return BorderStyle.Render("│") + treePart + pad + midSep + timelineBar + BorderStyle.Render("│")
	} else if isSearchMatch {
		pad := SearchRowBgStyle.Render(strings.Repeat(" ", treePadding))
		return BorderStyle.Render("│") + treePart + pad + midSep + timelineBar + BorderStyle.Render("│")
	} else if isHidden {
		treePart += strings.Repeat(" ", treePadding)
		return BorderStyle.Render("│") + HiddenStyle.Render(treePart) + midSep + timelineBar + BorderStyle.Render("│")
	} else if isAfterEnd {
		treePart += strings.Repeat(" ", treePadding)
		return BorderStyle.Render("│") + treePart + midSep + timelineBar + BorderStyle.Render("│")
	}
	treePart += strings.Repeat(" ", treePadding)
	return BorderStyle.Render("│") + treePart + midSep + timelineBar + BorderStyle.Render("│")
}

// getItemIcon returns the icon for an item type.
// Uses hints.Icon when available, falling back to type-based defaults for
// synthetic items (URLGroup, ActivityGroup) that have no enrichment hints.
func getItemIcon(item TreeItem) string {
	switch item.ItemType {
	case ItemTypeURLGroup:
		return "🔗" // width 2
	case ItemTypeActivityGroup:
		if item.Hints.Icon != "" {
			return item.Hints.Icon
		}
		return "📌" // width 2
	default:
		if item.Hints.Icon != "" {
			return item.Hints.Icon
		}
		return "• " // width 1 + 1 space = 2
	}
}

// getStatusIcon returns the status icon based on outcome
func getStatusIcon(item TreeItem) string {
	switch item.Hints.Outcome {
	case "pending":
		return "◷"
	case "success":
		return "✓"
	case "failure":
		return "✗"
	case "skipped":
		return "○"
	default:
		return " "
	}
}

// getStyledStatusIcon returns the status icon with color applied
func getStyledStatusIcon(item TreeItem) string {
	switch item.Hints.Outcome {
	case "pending":
		return PendingStyle.Render("◷")
	case "success":
		return SuccessStyle.Render("✓")
	case "failure":
		return FailureStyle.Render("✗")
	case "skipped":
		return SkippedStyle.Render("○")
	default:
		return " "
	}
}

// getStyledStatusIconWithBg returns the status icon with color and background
func getStyledStatusIconWithBg(item TreeItem, bg lipgloss.Color) string {
	bgStyle := lipgloss.NewStyle().Background(bg)
	switch item.Hints.Outcome {
	case "pending":
		return PendingStyle.Background(bg).Render("◷")
	case "success":
		return SuccessStyle.Background(bg).Render("✓")
	case "failure":
		return FailureStyle.Background(bg).Render("✗")
	case "skipped":
		return SkippedStyle.Background(bg).Render("○")
	default:
		return bgStyle.Render(" ")
	}
}

// getBadges returns badges for required and bottleneck status
func getBadges(item TreeItem) string {
	badges := ""
	if item.Hints.IsRequired {
		badges += " 🔒"
	}
	if item.IsBottleneck {
		badges += " 🔥"
	}
	return badges
}

// getBadgesWidth calculates the width of badges using fixed emoji widths
func getBadgesWidth(badges string) int {
	width := 0
	for _, r := range badges {
		s := string(r)
		width += GetCharWidth(s)
	}
	return width
}

// renderFooter renders the help footer with context-sensitive keybindings
func (m Model) renderFooter() string {
	width := m.width
	if width < 40 {
		width = 40
	}
	totalWidth := width - horizontalPad*2 // account for left/right padding
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Determine help mode
	mode := HelpModeNormal
	if m.isSearching {
		mode = HelpModeSearch
	} else if m.searchQuery != "" {
		mode = HelpModeSearchActive
	} else if m.showDetailModal {
		mode = HelpModeModal
	}

	// Build left (help) and right (status indicators) content
	help := m.keys.ShortHelpForMode(mode)

	// Build right-side status indicators
	var indicators []string
	if m.sortMode != SortByStartTime {
		indicators = append(indicators, "sort:"+m.sortMode.String())
	}
	if m.treeWidth != defaultTreeWidth {
		indicators = append(indicators, fmt.Sprintf("tree:%d", m.treeWidth))
	}


	right := ""
	rightPlain := ""
	if len(indicators) > 0 {
		rightPlain = " " + strings.Join(indicators, " • ") + " "
		right = " " + FooterStyle.Render(strings.Join(indicators, " • ")) + " "
	}
	rightWidth := lipgloss.Width(rightPlain)

	helpWidth := lipgloss.Width(help)
	contentWidth := totalWidth - 2 // account for "│" prefix and "│" suffix

	// Center help text in remaining space
	availableForHelp := contentWidth - rightWidth
	leftPadding := (availableForHelp - helpWidth) / 2
	if leftPadding < 0 {
		leftPadding = 0
	}
	midPadding := availableForHelp - helpWidth - leftPadding
	if midPadding < 0 {
		midPadding = 0
	}

	helpLine := BorderStyle.Render("│") + strings.Repeat(" ", leftPadding) + FooterStyle.Render(help) + strings.Repeat(" ", midPadding) + right + BorderStyle.Render("│")

	// Bottom border is a simple continuous line with rounded corners
	bottomBorder := BorderStyle.Render("╰" + strings.Repeat("─", max(0, totalWidth-2)) + "╯")

	return helpLine + "\n" + bottomBorder
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// placeModalCentered renders the modal centered on a dim background
func placeModalCentered(modal string, width, height int) string {
	modalLines := strings.Split(modal, "\n")

	// Get modal dimensions
	modalHeight := len(modalLines)
	modalWidth := 0
	for _, line := range modalLines {
		w := lipgloss.Width(line)
		if w > modalWidth {
			modalWidth = w
		}
	}

	// Calculate vertical padding to center
	topPadding := (height - modalHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	// Calculate horizontal padding to center
	leftPadding := (width - modalWidth) / 2
	if leftPadding < 0 {
		leftPadding = 0
	}

	// Build the output
	var result strings.Builder

	// Add top padding lines
	for i := 0; i < topPadding; i++ {
		result.WriteString(strings.Repeat(" ", width))
		result.WriteString("\n")
	}

	// Add modal lines with horizontal centering
	for _, line := range modalLines {
		lineWidth := lipgloss.Width(line)
		rightPadding := width - leftPadding - lineWidth
		if rightPadding < 0 {
			rightPadding = 0
		}
		result.WriteString(strings.Repeat(" ", leftPadding))
		result.WriteString(line)
		result.WriteString(strings.Repeat(" ", rightPadding))
		result.WriteString("\n")
	}

	// Add bottom padding to fill the screen
	linesWritten := topPadding + modalHeight
	for i := linesWritten; i < height; i++ {
		result.WriteString(strings.Repeat(" ", width))
		result.WriteString("\n")
	}

	return result.String()
}

// renderHelpModal renders the help modal with all key bindings
func (m Model) renderHelpModal() string {
	var b strings.Builder

	// Title
	b.WriteString(ModalTitleStyle.Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")

	// Key bindings
	keyStyle := lipgloss.NewStyle().Foreground(ColorBlue).Width(12)
	descStyle := lipgloss.NewStyle().Foreground(ColorWhite)

	for _, binding := range m.keys.FullHelp() {
		b.WriteString(keyStyle.Render(binding[0]))
		b.WriteString(descStyle.Render(binding[1]))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(FooterStyle.Render("Press Esc or ? to close"))

	return ModalStyle.Render(b.String())
}

// renderDetailModal renders the detail modal for an item
// Returns the rendered modal and the maximum scroll value
func (m Model) renderDetailModal(maxHeight, maxWidth int) (string, int) {
	if m.modalItem == nil {
		return "", 0
	}

	item := m.modalItem
	var lines []string

	// Helper to add a row
	addRow := func(label, value string) {
		lines = append(lines, ModalLabelStyle.Render(label)+ModalValueStyle.Render(value))
	}

	// Title
	lines = append(lines, ModalTitleStyle.Render("Item Details"))
	lines = append(lines, "")

	// Core Fields
	lines = append(lines, ModalTitleStyle.Render("── Core ──"))
	addRow("Name:", item.DisplayName)
	addRow("ID:", item.ID)
	if item.ParentID != "" {
		addRow("Parent ID:", item.ParentID)
	}
	addRow("Type:", item.ItemType.String())
	lines = append(lines, "")

	// Timing
	lines = append(lines, ModalTitleStyle.Render("── Timing ──"))
	if !item.StartTime.IsZero() {
		addRow("Start Time:", item.StartTime.Format("2006-01-02 15:04:05"))
	}
	if !item.EndTime.IsZero() {
		addRow("End Time:", item.EndTime.Format("2006-01-02 15:04:05"))
	}
	if !item.StartTime.IsZero() && !item.EndTime.IsZero() {
		duration := item.EndTime.Sub(item.StartTime).Seconds()
		if duration < 0 {
			duration = 0
		}
		addRow("Duration:", utils.HumanizeTime(duration))
	}
	lines = append(lines, "")

	// Status
	lines = append(lines, ModalTitleStyle.Render("── Status ──"))
	if item.Hints.Outcome != "" {
		addRow("Outcome:", item.Hints.Outcome)
	}
	if item.Hints.IsRequired {
		addRow("Is Required:", "Yes")
	} else {
		addRow("Is Required:", "No")
	}
	if item.IsBottleneck {
		addRow("Is Bottleneck:", "Yes")
	} else {
		addRow("Is Bottleneck:", "No")
	}
	lines = append(lines, "")

	// Links
	if item.Hints.URL != "" {
		lines = append(lines, ModalTitleStyle.Render("── Links ──"))
		linkedURL := hyperlink(item.Hints.URL, item.Hints.URL)
		lines = append(lines, ModalLabelStyle.Render("URL:")+linkedURL)
		lines = append(lines, "")
	}

	// Marker-specific fields
	if item.Hints.IsMarker {
		lines = append(lines, ModalTitleStyle.Render("── Marker ──"))
		if item.Hints.User != "" {
			addRow("User:", item.Hints.User)
		}
		if item.Hints.EventType != "" {
			addRow("Event Type:", item.Hints.EventType)
		}
		lines = append(lines, "")
	}

	// OTel Identity
	if item.TraceID != "" || item.SpanID != "" {
		lines = append(lines, ModalTitleStyle.Render("── Trace Identity ──"))
		if item.TraceID != "" {
			addRow("Trace ID:", item.TraceID)
		}
		if item.SpanID != "" {
			addRow("Span ID:", item.SpanID)
		}
		lines = append(lines, "")
	}

	// Service context from enrichment
	if item.Hints.ServiceName != "" || item.Hints.Environment != "" || item.Hints.Detail != "" {
		lines = append(lines, ModalTitleStyle.Render("── Context ──"))
		if item.Hints.ServiceName != "" {
			addRow("Service:", item.Hints.ServiceName)
		}
		if item.Hints.Environment != "" {
			addRow("Environment:", item.Hints.Environment)
		}
		if item.Hints.Detail != "" {
			addRow("Detail:", item.Hints.Detail)
		}
		if item.Hints.VCSBranch != "" {
			addRow("Branch:", item.Hints.VCSBranch)
		}
		if item.Hints.VCSRevision != "" {
			rev := item.Hints.VCSRevision
			if len(rev) > 12 {
				rev = rev[:12]
			}
			addRow("Revision:", rev)
		}
		if item.Hints.RunID != "" {
			addRow("Run ID:", item.Hints.RunID)
		}
		lines = append(lines, "")
	}

	// InstrumentationScope
	if item.ScopeName != "" {
		lines = append(lines, ModalTitleStyle.Render("── Instrumentation ──"))
		addRow("Library:", item.ScopeName)
		if item.ScopeVersion != "" {
			addRow("Version:", item.ScopeVersion)
		}
		lines = append(lines, "")
	}

	// Span Events (exceptions, logs)
	if len(item.Events) > 0 {
		lines = append(lines, ModalTitleStyle.Render("── Events ──"))
		for _, ev := range item.Events {
			timeStr := ""
			if !ev.Time.IsZero() {
				timeStr = " @ " + ev.Time.Format("15:04:05")
			}
			lines = append(lines, ModalValueStyle.Render(fmt.Sprintf("  %s%s", ev.Name, timeStr)))
			// Show exception details prominently
			if exType := ev.Attrs["exception.type"]; exType != "" {
				addRow("    Type:", exType)
			}
			if exMsg := ev.Attrs["exception.message"]; exMsg != "" {
				msg := exMsg
				if len(msg) > 120 {
					msg = msg[:117] + "..."
				}
				addRow("    Message:", msg)
			}
			if stack := ev.Attrs["exception.stacktrace"]; stack != "" {
				// Show first 3 lines of stacktrace
				stackLines := strings.SplitN(stack, "\n", 4)
				for i, sl := range stackLines {
					if i >= 3 {
						addRow("    ...", fmt.Sprintf("(%d more lines)", strings.Count(stack, "\n")-3))
						break
					}
					if sl != "" {
						addRow("   ", sl)
					}
				}
			}
			// Show other event attributes
			for k, v := range ev.Attrs {
				if k != "exception.type" && k != "exception.message" && k != "exception.stacktrace" && v != "" {
					addRow("    "+k+":", v)
				}
			}
		}
		lines = append(lines, "")
	}

	// Span Links (cross-trace references)
	if len(item.Links) > 0 {
		lines = append(lines, ModalTitleStyle.Render("── Links ──"))
		for i, link := range item.Links {
			lines = append(lines, ModalValueStyle.Render(fmt.Sprintf("  Link %d:", i+1)))
			addRow("    Trace ID:", link.TraceID)
			addRow("    Span ID:", link.SpanID)
			for k, v := range link.Attrs {
				if v != "" {
					addRow("    "+k+":", v)
				}
			}
		}
		lines = append(lines, "")
	}

	// Span Attributes (from source node)
	if item.sourceNode != nil && len(item.sourceNode.Attrs) > 0 {
		lines = append(lines, ModalTitleStyle.Render("── Attributes ──"))
		// Sort keys for stable display
		keys := make([]string, 0, len(item.sourceNode.Attrs))
		for k := range item.sourceNode.Attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := item.sourceNode.Attrs[k]
			if v != "" {
				addRow(k+":", v)
			}
		}
		lines = append(lines, "")
	}

	// Resource Attributes
	if len(item.ResourceAttrs) > 0 {
		lines = append(lines, ModalTitleStyle.Render("── Resource ──"))
		keys := make([]string, 0, len(item.ResourceAttrs))
		for k := range item.ResourceAttrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := item.ResourceAttrs[k]
			if v != "" {
				addRow(k+":", v)
			}
		}
		lines = append(lines, "")
	}

	// Tree Info
	lines = append(lines, ModalTitleStyle.Render("── Tree ──"))
	addRow("Depth:", fmt.Sprintf("%d", item.Depth))
	addRow("Has Children:", fmt.Sprintf("%v", item.HasChildren))
	if item.HasChildren {
		addRow("Child Count:", fmt.Sprintf("%d", len(item.Children)))
	}

	// Calculate available height for content (account for border and padding)
	// ModalStyle has Padding(1, 2) = 1 top, 1 bottom, 2 left, 2 right
	// Plus 2 for the border itself
	contentMaxHeight := maxHeight - 4 // 2 for border, 2 for padding

	// Reserve 2 lines for footer
	contentMaxHeight -= 2

	if contentMaxHeight < 5 {
		contentMaxHeight = 5
	}

	// Calculate max scroll
	totalLines := len(lines)
	maxScroll := totalLines - contentMaxHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Apply scroll
	scroll := m.modalScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Calculate max content width from ALL lines (not just visible)
	maxContentWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxContentWidth {
			maxContentWidth = w
		}
	}

	// Get visible lines
	endIdx := scroll + contentMaxHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}
	visibleLines := lines[scroll:endIdx]

	// Build content without scrollbar (scrollbar added outside modal border)
	var b strings.Builder
	showScrollbar := maxScroll > 0
	visibleCount := len(visibleLines)

	// Pad each visible line to max content width for consistent modal width
	for _, line := range visibleLines {
		lineWidth := lipgloss.Width(line)
		b.WriteString(line)
		if lineWidth < maxContentWidth {
			b.WriteString(strings.Repeat(" ", maxContentWidth-lineWidth))
		}
		b.WriteString("\n")
	}

	// Footer hint with scroll indicator (padded to match content width)
	b.WriteString("\n")
	var footerText string
	if maxScroll > 0 {
		scrollInfo := fmt.Sprintf("[%d/%d] ", scroll+1, maxScroll+1)
		footerText = FooterStyle.Render(scrollInfo + "↑↓ scroll • ←→ prev/next • Esc close")
	} else {
		footerText = FooterStyle.Render("←→ prev/next • Esc/i close")
	}
	footerWidth := lipgloss.Width(footerText)
	b.WriteString(footerText)
	if footerWidth < maxContentWidth {
		b.WriteString(strings.Repeat(" ", maxContentWidth-footerWidth))
	}

	// Apply max width constraint to the style
	modalStyle := ModalStyle.MaxWidth(maxWidth)
	content := b.String()
	renderedModal := modalStyle.Render(content)

	// Add scrollbar outside the modal border if needed
	if showScrollbar {
		renderedModal = addScrollbarToModal(renderedModal, scroll, maxScroll, visibleCount, totalLines)
	}

	return renderedModal, maxScroll
}

// addScrollbarToModal adds a scrollbar column to the right of the modal border
// The scrollbar is 80% of the modal height, vertically centered
func addScrollbarToModal(modal string, scroll, maxScroll, visibleCount, totalLines int) string {
	lines := strings.Split(modal, "\n")
	if len(lines) == 0 {
		return modal
	}

	// Calculate scrollbar track dimensions (80% height, centered)
	totalHeight := len(lines)
	trackHeight := totalHeight * 80 / 100
	if trackHeight < 3 {
		trackHeight = min(3, totalHeight)
	}
	topPadding := (totalHeight - trackHeight) / 2
	bottomPadding := totalHeight - trackHeight - topPadding

	// Calculate thumb size and position within the track
	thumbSize := max(1, trackHeight*visibleCount/totalLines)
	if thumbSize > trackHeight {
		thumbSize = trackHeight
	}

	thumbStart := 0
	if maxScroll > 0 {
		thumbStart = scroll * (trackHeight - thumbSize) / maxScroll
	}
	thumbEnd := thumbStart + thumbSize

	// Scrollbar style (subtle, matches separator)
	scrollStyle := lipgloss.NewStyle().Foreground(ColorGrayDim)
	thumbChar := scrollStyle.Render("┃")
	trackChar := scrollStyle.Render("│")

	var result strings.Builder
	for i, line := range lines {
		result.WriteString(line)

		// Determine scrollbar character for this line
		trackIndex := i - topPadding
		if i < topPadding || i >= totalHeight-bottomPadding {
			// Outside track area - no scrollbar character, just space
			result.WriteString(" ")
		} else if trackIndex >= thumbStart && trackIndex < thumbEnd {
			result.WriteString(thumbChar)
		} else {
			result.WriteString(trackChar)
		}

		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
