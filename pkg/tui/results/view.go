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
	iconWidth := GetCharWidth(icon)

	// Get status indicator
	statusIcon := getStatusIcon(item)
	statusWidth := GetCharWidth(statusIcon)

	// Get badges
	badges := getBadges(item)
	badgesWidth := getBadgesWidth(badges)

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
	if isSelected && isHidden {
		// Hidden and selected: gray text with selection background
		return BorderStyle.Render("│") + HiddenSelectedStyle.Render(treePart) + BorderStyle.Render("│") + HiddenSelectedStyle.Render(timelineBar) + BorderStyle.Render("│")
	} else if isSelected {
		// Selected but not hidden: white text with selection background
		return BorderStyle.Render("│") + SelectedStyle.Render(treePart) + BorderStyle.Render("│") + SelectedStyle.Render(timelineBar) + BorderStyle.Render("│")
	} else if isHidden {
		// Hidden but not selected: gray text, no background
		return BorderStyle.Render("│") + HiddenStyle.Render(treePart) + BorderStyle.Render("│") + timelineBar + BorderStyle.Render("│")
	}
	// Normal: no special styling
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

// getBadgesWidth calculates the width of badges using fixed emoji widths
func getBadgesWidth(badges string) int {
	width := 0
	for _, r := range badges {
		s := string(r)
		width += GetCharWidth(s)
	}
	return width
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
		addRow("Duration:", utils.HumanizeTime(duration))
	}
	lines = append(lines, "")

	// Status
	lines = append(lines, ModalTitleStyle.Render("── Status ──"))
	if item.Status != "" {
		addRow("Status:", item.Status)
	}
	if item.Conclusion != "" {
		addRow("Conclusion:", item.Conclusion)
	}
	if item.IsRequired {
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
	if item.URL != "" {
		lines = append(lines, ModalTitleStyle.Render("── Links ──"))
		linkedURL := hyperlink(item.URL, item.URL)
		lines = append(lines, ModalLabelStyle.Render("URL:")+linkedURL)
		lines = append(lines, "")
	}

	// Marker-specific fields
	if item.ItemType == ItemTypeMarker {
		lines = append(lines, ModalTitleStyle.Render("── Marker ──"))
		if item.User != "" {
			addRow("User:", item.User)
		}
		if item.EventType != "" {
			addRow("Event Type:", item.EventType)
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

	// Scrollbar style (same color as modal border)
	scrollStyle := lipgloss.NewStyle().Foreground(ColorGrayLight)
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
