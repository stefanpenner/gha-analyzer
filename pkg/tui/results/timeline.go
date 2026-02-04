package results

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// RenderTimelineBar renders a timeline bar for a tree item
func RenderTimelineBar(item TreeItem, globalStart, globalEnd time.Time, width int) string {
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

	if itemEnd.Before(itemStart) || itemEnd.Equal(itemStart) {
		return strings.Repeat(" ", width)
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

	return leftPad + style.Render(bar) + rightPad
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

	// Markers use point markers
	if item.ItemType == ItemTypeMarker {
		switch item.EventType {
		case "merged":
			return "◆", BarSuccessStyle
		case "approved":
			return "▲", BarPendingStyle
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
