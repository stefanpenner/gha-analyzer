package results

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
)

const (
	treeWidth     = 55
	timelineWidth = 30
)

// hyperlink wraps text in OSC 8 terminal hyperlink escape sequence.
// This makes the text clickable in supporting terminals (iTerm2, Windows Terminal, etc.)
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	// OSC 8 format: \x1b]8;;URL\x07TEXT\x1b]8;;\x07
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, text)
}

// renderHeader renders the title bar with statistics
func (m Model) renderHeader() string {
	totalWidth := m.width
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Build borders
	topBorder := "┌" + strings.Repeat("─", max(0, totalWidth-2)) + "┐"
	bottomBorder := "└" + strings.Repeat("─", max(0, totalWidth-2)) + "┘"

	// Line 1: Title
	titleText := "GitHub Actions Analyzer"
	title := HeaderStyle.Render(titleText)
	titleWidth := lipgloss.Width(titleText)
	titlePadding := totalWidth - titleWidth - 4
	if titlePadding < 0 {
		titlePadding = 0
	}
	line1 := "│ " + title + strings.Repeat(" ", titlePadding) + " │"

	// Line 2: Success rate and concurrency
	successRate := float64(0)
	if m.summary.TotalRuns > 0 {
		successRate = float64(m.summary.SuccessfulRuns) / float64(m.summary.TotalRuns) * 100
	}
	jobSuccessRate := float64(0)
	if m.summary.TotalJobs > 0 {
		jobSuccessRate = float64(m.summary.TotalJobs-m.summary.FailedJobs) / float64(m.summary.TotalJobs) * 100
	}
	statsText := fmt.Sprintf("Success: %.0f%% workflows, %.0f%% jobs • Peak Concurrency: %d",
		successRate, jobSuccessRate, m.summary.MaxConcurrency)
	statsWidth := lipgloss.Width(statsText)
	statsPadding := totalWidth - statsWidth - 4
	if statsPadding < 0 {
		statsPadding = 0
	}
	line2 := "│ " + HeaderCountStyle.Render(statsText) + strings.Repeat(" ", statsPadding) + " │"

	// Line 3: Counts and times
	wallTime := utils.HumanizeTime(float64(m.wallTimeMs) / 1000)
	computeTime := utils.HumanizeTime(float64(m.computeMs) / 1000)
	countsText := fmt.Sprintf("%d runs • %d jobs • %d steps • wall: %s • compute: %s",
		m.summary.TotalRuns, m.summary.TotalJobs, m.stepCount, wallTime, computeTime)
	countsWidth := lipgloss.Width(countsText)
	countsPadding := totalWidth - countsWidth - 4
	if countsPadding < 0 {
		countsPadding = 0
	}
	line3 := "│ " + HeaderCountStyle.Render(countsText) + strings.Repeat(" ", countsPadding) + " │"

	// Line 4: Source URL (if available)
	line4 := ""
	if m.sourceURL != "" {
		urlText := m.sourceURL
		if m.sourceName != "" {
			urlText = m.sourceName + ": " + m.sourceURL
		}
		// Truncate if too long
		maxURLWidth := totalWidth - 6
		if lipgloss.Width(urlText) > maxURLWidth {
			urlText = urlText[:maxURLWidth-3] + "..."
		}
		// Make it a clickable hyperlink
		linkedURL := hyperlink(m.sourceURL, urlText)
		urlWidth := lipgloss.Width(urlText)
		urlPadding := totalWidth - urlWidth - 4
		if urlPadding < 0 {
			urlPadding = 0
		}
		line4 = "\n│ " + linkedURL + strings.Repeat(" ", urlPadding) + " │"
	}

	return topBorder + "\n" + line1 + "\n" + line2 + "\n" + line3 + line4 + "\n" + bottomBorder
}

// renderTimeHeader renders the time scale header
func (m Model) renderTimeHeader() string {
	if m.globalStart.IsZero() || m.globalEnd.IsZero() {
		return ""
	}

	startTime := m.globalStart.Format("15:04:05")
	endTime := m.globalEnd.Format("15:04:05")
	duration := utils.HumanizeTime(m.globalEnd.Sub(m.globalStart).Seconds())

	totalWidth := m.width
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Calculate available width for the timeline part
	availableWidth := totalWidth - 4 // borders and padding

	// Build time info string with labels
	timeInfo := fmt.Sprintf("Start: %s │ Duration: %s │ End: %s", startTime, duration, endTime)
	timeInfoWidth := lipgloss.Width(timeInfo)

	leftPad := (availableWidth - timeInfoWidth) / 2
	rightPad := availableWidth - timeInfoWidth - leftPad
	if leftPad < 0 {
		leftPad = 0
	}
	if rightPad < 0 {
		rightPad = 0
	}

	header := "│" + strings.Repeat(" ", leftPad) + timeInfo + strings.Repeat(" ", rightPad) + "│"
	border := "├" + strings.Repeat("─", max(0, totalWidth-2)) + "┤"

	return TimeHeaderStyle.Render(header) + "\n" + BorderStyle.Render(border)
}

// renderItem renders a single tree item with timeline bar
func (m Model) renderItem(item TreeItem, isSelected bool) string {
	totalWidth := m.width
	if totalWidth < 1 {
		totalWidth = 80
	}

	// Calculate widths
	// Line structure: │ + treePart + │ + timelineBar + │ = 3 border chars
	availableWidth := totalWidth - 3 // 3 border characters
	treeW := treeWidth
	timelineW := availableWidth - treeW
	if timelineW < 10 {
		timelineW = 10
	}

	// Build indent
	// Steps align under their parent job icon, so use parent's depth
	indentDepth := item.Depth
	if item.ItemType == ItemTypeStep && indentDepth > 0 {
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
	iconWidth := lipgloss.Width(icon)

	// Get status indicator
	statusIcon := getStatusIcon(item)
	statusWidth := lipgloss.Width(statusIcon)

	// Get badges
	badges := getBadges(item)
	badgesWidth := lipgloss.Width(badges)

	// Build the name part with duration
	name := item.DisplayName
	if item.User != "" && item.ItemType == ItemTypeMarker {
		name = fmt.Sprintf("%s by %s", name, item.User)
	}
	// Add duration in parentheses
	if !item.StartTime.IsZero() && !item.EndTime.IsZero() {
		duration := item.EndTime.Sub(item.StartTime).Seconds()
		name = fmt.Sprintf("%s (%s)", name, utils.HumanizeTime(duration))
	}

	// Calculate available space for name
	// Format: indent + expand + space + icon + space + name + badges + space + status
	usedWidth := indentWidth + expandWidth + 1 + iconWidth + 1 + badgesWidth + 1 + statusWidth
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
	// Format: indent + expand + space + icon + space + name + badges + space + status
	treePartWidth := indentWidth + expandWidth + 1 + iconWidth + 1 + nameWidth + badgesWidth + 1 + statusWidth

	// Pad tree part to fixed width
	treePadding := treeW - treePartWidth
	if treePadding < 0 {
		treePadding = 0
	}

	// Wrap name in hyperlink if URL is available (must be done after width calculation)
	displayName := hyperlink(item.URL, name)

	// Build tree part with hyperlinked name
	treePart := fmt.Sprintf("%s%s %s %s%s %s", indent, expandIndicator, icon, displayName, badges, statusIcon)
	treePart += strings.Repeat(" ", treePadding)

	// Render timeline bar
	timelineBar := RenderTimelineBar(item, m.globalStart, m.globalEnd, timelineW)

	// Combine
	line := "│" + treePart + "│" + timelineBar + "│"

	// Apply selection style
	if isSelected {
		return SelectedStyle.Render(line)
	}
	return line
}

// getItemIcon returns the icon for an item type
// All icons are normalized to display width 2 for consistent alignment
func getItemIcon(item TreeItem) string {
	switch item.ItemType {
	case ItemTypeWorkflow:
		return "📋" // width 2
	case ItemTypeJob:
		return "⚙️" // width 2
	case ItemTypeStep:
		return "↳" // width 1, no padding needed for steps
	case ItemTypeMarker:
		switch item.EventType {
		case "merged":
			return "◆ " // width 1 + 1 space = 2
		case "approved":
			return "▲ " // width 1 + 1 space = 2
		case "comment", "commented":
			return "💬" // width 2
		case "changes_requested":
			return "❌" // width 2
		default:
			return "▲ " // width 1 + 1 space = 2
		}
	default:
		return "• " // width 1 + 1 space = 2
	}
}

// getStatusIcon returns the status icon based on conclusion
func getStatusIcon(item TreeItem) string {
	switch item.Status {
	case "in_progress", "queued", "waiting":
		return "◷"
	}

	switch item.Conclusion {
	case "success":
		return "✓"
	case "failure":
		return "✗"
	case "skipped", "cancelled":
		return "○"
	default:
		return " "
	}
}

// getBadges returns badges for required and bottleneck status
func getBadges(item TreeItem) string {
	badges := ""
	if item.IsRequired {
		badges += " 🔒"
	}
	if item.IsBottleneck {
		badges += " 🔥"
	}
	return badges
}

// renderFooter renders the help footer
func (m Model) renderFooter() string {
	totalWidth := m.width
	if totalWidth < 1 {
		totalWidth = 80
	}

	border := "├" + strings.Repeat("─", max(0, totalWidth-2)) + "┤"
	help := m.keys.ShortHelp()
	helpWidth := lipgloss.Width(help)

	// Center the help text
	leftPadding := (totalWidth - helpWidth - 4) / 2
	if leftPadding < 0 {
		leftPadding = 0
	}
	rightPadding := totalWidth - helpWidth - leftPadding - 4
	if rightPadding < 0 {
		rightPadding = 0
	}

	helpLine := "│" + strings.Repeat(" ", leftPadding) + help + strings.Repeat(" ", rightPadding) + " │"
	bottomBorder := "└" + strings.Repeat("─", max(0, totalWidth-2)) + "┘"

	return BorderStyle.Render(border) + "\n" + FooterStyle.Render(helpLine) + "\n" + BorderStyle.Render(bottomBorder)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
