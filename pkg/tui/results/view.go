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

	// Calculate widths to match item rows
	treeW := treeWidth
	availableW := totalWidth - 3 // 3 border chars
	timelineW := availableW - treeW
	if timelineW < 10 {
		timelineW = 10
	}

	// Build borders (all in gray, rounded corners)
	topBorder := BorderStyle.Render("╭" + strings.Repeat("─", max(0, totalWidth-2)) + "╮")
	// Bottom border transitions to the main content with column separator
	bottomBorder := BorderStyle.Render("├" + strings.Repeat("─", treeW) + "┬" + strings.Repeat("─", timelineW) + "┤")

	// Line 1: Title
	titleText := "GitHub Actions Analyzer"
	title := HeaderStyle.Render(titleText)
	titleWidth := lipgloss.Width(titleText)
	titlePadding := totalWidth - titleWidth - 4
	if titlePadding < 0 {
		titlePadding = 0
	}
	line1 := BorderStyle.Render("│") + " " + title + strings.Repeat(" ", titlePadding) + " " + BorderStyle.Render("│")

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
	line2 := BorderStyle.Render("│") + " " + HeaderCountStyle.Render(statsText) + strings.Repeat(" ", statsPadding) + " " + BorderStyle.Render("│")

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
	line3 := BorderStyle.Render("│") + " " + HeaderCountStyle.Render(countsText) + strings.Repeat(" ", countsPadding) + " " + BorderStyle.Render("│")

	// Line 4: Input URLs (clickable links)
	line4 := ""
	for _, inputURL := range m.inputURLs {
		// Truncate if too long
		urlText := inputURL
		maxURLWidth := totalWidth - 6
		if lipgloss.Width(urlText) > maxURLWidth {
			urlText = urlText[:maxURLWidth-3] + "..."
		}
		// Make it a clickable hyperlink
		linkedURL := hyperlink(inputURL, urlText)
		urlWidth := lipgloss.Width(urlText)
		urlPadding := totalWidth - urlWidth - 4
		if urlPadding < 0 {
			urlPadding = 0
		}
		line4 += "\n" + BorderStyle.Render("│") + " " + linkedURL + strings.Repeat(" ", urlPadding) + " " + BorderStyle.Render("│")
	}

	// Line 5: Time range info
	line5 := ""
	if !m.chartStart.IsZero() && !m.chartEnd.IsZero() {
		startTime := m.chartStart.Format("15:04:05")
		endTime := m.chartEnd.Format("15:04:05")
		duration := utils.HumanizeTime(m.chartEnd.Sub(m.chartStart).Seconds())
		timeText := fmt.Sprintf("Start: %s • Duration: %s • End: %s", startTime, duration, endTime)
		timeWidth := lipgloss.Width(timeText)
		timePadding := totalWidth - timeWidth - 4
		if timePadding < 0 {
			timePadding = 0
		}
		line5 = "\n" + BorderStyle.Render("│") + " " + HeaderCountStyle.Render(timeText) + strings.Repeat(" ", timePadding) + " " + BorderStyle.Render("│")
	}

	return topBorder + "\n" + line1 + "\n" + line2 + "\n" + line3 + line4 + line5 + "\n" + bottomBorder
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

	// Check if item is hidden from chart
	isHidden := m.hiddenState[item.ID]

	// Wrap name in hyperlink if URL is available (must be done after width calculation)
	displayName := hyperlink(item.URL, name)

	// Build tree part with hyperlinked name
	treePart := fmt.Sprintf("%s%s %s %s%s %s", indent, expandIndicator, icon, displayName, badges, statusIcon)
	treePart += strings.Repeat(" ", treePadding)

	// Apply gray style to tree part if hidden (but not if selected)
	if isHidden && !isSelected {
		treePart = HiddenStyle.Render(treePart)
	}

	// Render timeline bar (empty if hidden, unstyled if selected for consistent background)
	// URL is passed to render functions so only the bar/marker characters are clickable
	var timelineBar string
	if isHidden {
		timelineBar = strings.Repeat(" ", timelineW)
	} else if isSelected {
		// Render without colors so selection background shows through
		timelineBar = RenderTimelineBarPlain(item, m.chartStart, m.chartEnd, timelineW, item.URL)
	} else {
		timelineBar = RenderTimelineBar(item, m.chartStart, m.chartEnd, timelineW, item.URL)
	}

	// Combine with styled borders (borders always gray)
	if isSelected {
		// For selected items, apply selection style to content but keep borders gray
		return BorderStyle.Render("│") + SelectedStyle.Render(treePart) + BorderStyle.Render("│") + SelectedStyle.Render(timelineBar) + BorderStyle.Render("│")
	}
	// For non-selected items, apply border style to │ characters
	return BorderStyle.Render("│") + treePart + BorderStyle.Render("│") + timelineBar + BorderStyle.Render("│")
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

	// Calculate widths to match item rows
	treeW := treeWidth
	availableW := totalWidth - 3
	timelineW := availableW - treeW
	if timelineW < 10 {
		timelineW = 10
	}

	// Top border closes the column separator with ┴
	topBorder := "├" + strings.Repeat("─", treeW) + "┴" + strings.Repeat("─", timelineW) + "┤"

	help := m.keys.ShortHelp()
	helpWidth := lipgloss.Width(help)

	// Center the help text across full width (matching item row structure)
	contentWidth := totalWidth - 2 // account for "│" prefix and "│" suffix
	leftPadding := (contentWidth - helpWidth) / 2
	if leftPadding < 0 {
		leftPadding = 0
	}
	rightPadding := contentWidth - helpWidth - leftPadding
	if rightPadding < 0 {
		rightPadding = 0
	}

	helpLine := BorderStyle.Render("│") + strings.Repeat(" ", leftPadding) + FooterStyle.Render(help) + strings.Repeat(" ", rightPadding) + BorderStyle.Render("│")

	// Bottom border is a simple continuous line with rounded corners
	bottomBorder := BorderStyle.Render("╰" + strings.Repeat("─", max(0, totalWidth-2)) + "╯")

	return BorderStyle.Render(topBorder) + "\n" + helpLine + "\n" + bottomBorder
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
