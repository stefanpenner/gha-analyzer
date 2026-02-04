package results

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// timelineHyperlink wraps text in OSC 8 hyperlink with underline disabled.
func timelineHyperlink(url, text string) string {
	if url == "" {
		return text
	}
	// \x1b[24m disables underline
	return fmt.Sprintf("\x1b]8;;%s\x07\x1b[24m%s\x1b[24m\x1b]8;;\x07", url, text)
}

// renderMarker renders a marker character with proper padding, handling width consistently.
// Returns left padding + styled marker + right padding, totaling exactly 'width' visual characters.
func renderMarker(markerChar string, style lipgloss.Style, startPos, width int, url string, applyStyle bool) string {
	// Use a fixed width for known markers to avoid terminal inconsistencies
	markerWidth := getMarkerWidth(markerChar)

	// Clamp position
	if startPos < 0 {
		startPos = 0
	}
	if startPos > width-markerWidth {
		startPos = width - markerWidth
	}
	if startPos < 0 {
		startPos = 0
	}

	leftPadCount := startPos
	rightPadCount := width - startPos - markerWidth
	if rightPadCount < 0 {
		rightPadCount = 0
	}

	// Build the content (styled marker with hyperlink)
	var styledMarker string
	if applyStyle {
		styledMarker = style.Render(markerChar)
	} else {
		styledMarker = markerChar
	}
	content := timelineHyperlink(url, styledMarker)

	// Build result with exact padding
	result := strings.Repeat(" ", leftPadCount) + content + strings.Repeat(" ", rightPadCount)

	// Validate and fix total width - measure only visible characters
	actualWidth := leftPadCount + markerWidth + rightPadCount
	if actualWidth < width {
		// Add missing spaces at end
		result += strings.Repeat(" ", width-actualWidth)
	}

	return result
}

// GetCharWidth returns the visual width of a character/emoji.
// Uses fixed values for known characters to ensure consistency across renders.
// This is exported so view.go can use it too.
func GetCharWidth(char string) int {
	switch char {
	case "💬", "📋", "⚙️", "❌":
		return 2
	case "◆", "✓", "✗", "▲", "|", "↳", "◷", "○", "▼", "▶", " ":
		return 1
	case "◆ ", "▲ ", "• ":
		return 2
	case "🔒", "🔥":
		return 2
	default:
		return lipgloss.Width(char)
	}
}

// getMarkerWidth returns the visual width of a marker character.
func getMarkerWidth(char string) int {
	return GetCharWidth(char)
}

// RenderTimelineBar renders a timeline bar for a tree item
func RenderTimelineBar(item TreeItem, globalStart, globalEnd time.Time, width int, url string) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return strings.Repeat(" ", width)
	}

	totalDuration := globalEnd.Sub(globalStart)

	// Calculate bar position
	itemStart := item.StartTime
	itemEnd := item.EndTime

	// Clamp to global bounds
	if itemStart.Before(globalStart) {
		itemStart = globalStart
	}
	if itemEnd.After(globalEnd) {
		itemEnd = globalEnd
	}

	// Handle 0-duration items (show as a marker at start position)
	isZeroDuration := itemEnd.Before(itemStart) || itemEnd.Equal(itemStart)
	if isZeroDuration {
		// Get character and style based on item type/status
		markerChar, style := getBarStyle(item)
		// For non-markers, use | as the zero-duration indicator
		if item.ItemType != ItemTypeMarker {
			markerChar = "|"
		}

		// Calculate position for the marker
		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))

		return renderMarker(markerChar, style, startPos, width, url, true)
	}

	startOffset := itemStart.Sub(globalStart)
	endOffset := itemEnd.Sub(globalStart)

	startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))
	endPos := int(float64(endOffset) / float64(totalDuration) * float64(width))

	// Ensure at least 1 character bar
	barLength := endPos - startPos
	if barLength < 1 {
		barLength = 1
	}

	// Clamp startPos to valid range [0, width-1]
	if startPos < 0 {
		startPos = 0
	}
	if startPos > width-1 {
		startPos = width - 1
	}

	// Clamp barLength to fit within remaining space
	if startPos+barLength > width {
		barLength = width - startPos
	}
	if barLength < 1 {
		barLength = 1
	}

	// Choose bar character and style based on status
	barChar, style := getBarStyle(item)

	// Build the bar - rightPad is guaranteed non-negative now
	leftPad := strings.Repeat(" ", startPos)
	bar := strings.Repeat(barChar, barLength)
	rightPad := strings.Repeat(" ", width-startPos-barLength)

	// Wrap only the bar in hyperlink
	styledBar := style.Render(bar)
	return leftPad + timelineHyperlink(url, styledBar) + rightPad
}

// RenderTimelineBarPlain renders a timeline bar without colors (for selected items)
func RenderTimelineBarPlain(item TreeItem, globalStart, globalEnd time.Time, width int, url string) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return strings.Repeat(" ", width)
	}

	totalDuration := globalEnd.Sub(globalStart)

	// Calculate bar position
	itemStart := item.StartTime
	itemEnd := item.EndTime

	// Clamp to global bounds
	if itemStart.Before(globalStart) {
		itemStart = globalStart
	}
	if itemEnd.After(globalEnd) {
		itemEnd = globalEnd
	}

	// Handle 0-duration items
	isZeroDuration := itemEnd.Before(itemStart) || itemEnd.Equal(itemStart)
	if isZeroDuration {
		markerChar, style := getBarStyle(item)
		if item.ItemType != ItemTypeMarker {
			markerChar = "|"
		}

		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))

		return renderMarker(markerChar, style, startPos, width, url, false)
	}

	startOffset := itemStart.Sub(globalStart)
	endOffset := itemEnd.Sub(globalStart)

	startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))
	endPos := int(float64(endOffset) / float64(totalDuration) * float64(width))

	barLength := endPos - startPos
	if barLength < 1 {
		barLength = 1
	}

	if startPos < 0 {
		startPos = 0
	}
	if startPos > width-1 {
		startPos = width - 1
	}
	if startPos+barLength > width {
		barLength = width - startPos
	}
	if barLength < 1 {
		barLength = 1
	}

	barChar, _ := getBarStyle(item)

	leftPad := strings.Repeat(" ", startPos)
	bar := strings.Repeat(barChar, barLength)
	rightPad := strings.Repeat(" ", width-startPos-barLength)

	// Wrap only the bar in hyperlink
	return leftPad + timelineHyperlink(url, bar) + rightPad
}

// getBarStyle returns the bar character and style based on item status
func getBarStyle(item TreeItem) (string, lipgloss.Style) {
	// Steps use different character
	if item.ItemType == ItemTypeStep {
		switch item.Conclusion {
		case "success":
			return "▒", BarSuccessStyle
		case "failure":
			return "▒", BarFailureStyle
		case "skipped", "cancelled":
			return "░", BarSkippedStyle
		default:
			return "▒", BarPendingStyle
		}
	}

	// Markers use point markers (single-width chars for consistent timeline rendering)
	if item.ItemType == ItemTypeMarker {
		switch item.EventType {
		case "merged":
			return "◆", BarSuccessStyle
		case "approved":
			return "✓", BarSuccessStyle
		case "comment", "commented", "COMMENTED":
			return "○", BarPendingStyle // Use ○ instead of 💬 for consistent width
		case "changes_requested":
			return "✗", BarFailureStyle
		default:
			return "▲", BarPendingStyle
		}
	}

	// Jobs and workflows
	switch {
	case item.Status == "in_progress" || item.Status == "queued" || item.Status == "waiting":
		return "▒", BarPendingStyle
	case item.Conclusion == "success":
		return "█", BarSuccessStyle
	case item.Conclusion == "failure":
		if item.IsRequired {
			return "█", BarFailureStyle
		}
		return "░", BarFailureNonBlockingStyle
	case item.Conclusion == "skipped" || item.Conclusion == "cancelled":
		return "░", BarSkippedStyle
	default:
		// Unknown/empty conclusion - use gray like non-TUI
		return "░", BarSkippedStyle
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
