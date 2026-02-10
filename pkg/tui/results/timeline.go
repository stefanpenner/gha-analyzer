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
	// id parameter ensures terminals treat each link independently
	return fmt.Sprintf("\x1b]8;id=%s;%s\x07\x1b[24m%s\x1b[24m\x1b]8;;\x07", url, url, text)
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
	case "📋", "⚙️", "❌":
		return 2
	case "◆", "✓", "✗", "▲", "|", "↳", "◷", "○", "▼", "▶", " ", "●":
		return 1
	case "◆ ", "▲ ", "• ", "● ":
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

// RenderTimelineBarSelected renders a timeline bar with dimmed colors and selection background
func RenderTimelineBarSelected(item TreeItem, globalStart, globalEnd time.Time, width int, url string) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return SelectedBgStyle.Render(strings.Repeat(" ", width))
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
		markerChar, style := getBarStyleSelected(item)
		if item.ItemType != ItemTypeMarker {
			markerChar = "|"
		}

		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))

		return renderMarker(markerChar, style, startPos, width, url, true)
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

	barChar, style := getBarStyleSelected(item)

	// Apply selection background to padding and bar
	leftPad := SelectedBgStyle.Render(strings.Repeat(" ", startPos))
	bar := strings.Repeat(barChar, barLength)
	rightPad := SelectedBgStyle.Render(strings.Repeat(" ", width-startPos-barLength))

	styledBar := style.Render(bar)
	return leftPad + timelineHyperlink(url, styledBar) + rightPad
}

// renderTimelineBarWithBg renders a timeline bar with normal colors but applies
// bgStyle to the empty space (left/right padding) for a subtle row tint.
func renderTimelineBarWithBg(item TreeItem, globalStart, globalEnd time.Time, width int, url string, bgStyle lipgloss.Style) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return bgStyle.Render(strings.Repeat(" ", width))
	}

	totalDuration := globalEnd.Sub(globalStart)
	itemStart := item.StartTime
	itemEnd := item.EndTime

	if itemStart.Before(globalStart) {
		itemStart = globalStart
	}
	if itemEnd.After(globalEnd) {
		itemEnd = globalEnd
	}

	isZeroDuration := itemEnd.Before(itemStart) || itemEnd.Equal(itemStart)
	if isZeroDuration {
		markerChar, style := getBarStyle(item)
		if item.ItemType != ItemTypeMarker {
			markerChar = "|"
		}
		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))
		// Render marker with bg on padding
		if startPos < 0 {
			startPos = 0
		}
		if startPos >= width {
			startPos = width - 1
		}
		leftPad := bgStyle.Render(strings.Repeat(" ", startPos))
		rightPad := bgStyle.Render(strings.Repeat(" ", width-startPos-1))
		return leftPad + style.Render(markerChar) + rightPad
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

	barChar, style := getBarStyle(item)

	leftPad := bgStyle.Render(strings.Repeat(" ", startPos))
	bar := strings.Repeat(barChar, barLength)
	rightPad := bgStyle.Render(strings.Repeat(" ", width-startPos-barLength))

	styledBar := style.Render(bar)
	return leftPad + timelineHyperlink(url, styledBar) + rightPad
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
			return "✓", BarSuccessStyle
		case "comment", "commented", "COMMENTED":
			return "●", BarPendingStyle
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

// getBarStyleSelected returns the bar character and dimmed style for selected items
func getBarStyleSelected(item TreeItem) (string, lipgloss.Style) {
	// Steps use different character
	if item.ItemType == ItemTypeStep {
		switch item.Conclusion {
		case "success":
			return "▒", BarSuccessSelectedStyle
		case "failure":
			return "▒", BarFailureSelectedStyle
		case "skipped", "cancelled":
			return "░", BarSkippedSelectedStyle
		default:
			return "▒", BarPendingSelectedStyle
		}
	}

	// Markers use point markers
	if item.ItemType == ItemTypeMarker {
		switch item.EventType {
		case "merged":
			return "◆", BarSuccessSelectedStyle
		case "approved":
			return "✓", BarSuccessSelectedStyle
		case "comment", "commented", "COMMENTED":
			return "●", BarPendingSelectedStyle
		case "changes_requested":
			return "✗", BarFailureSelectedStyle
		default:
			return "▲", BarPendingSelectedStyle
		}
	}

	// Jobs and workflows
	switch {
	case item.Status == "in_progress" || item.Status == "queued" || item.Status == "waiting":
		return "▒", BarPendingSelectedStyle
	case item.Conclusion == "success":
		return "█", BarSuccessSelectedStyle
	case item.Conclusion == "failure":
		if item.IsRequired {
			return "█", BarFailureSelectedStyle
		}
		return "░", BarFailureNonBlockingSelectedStyle
	case item.Conclusion == "skipped" || item.Conclusion == "cancelled":
		return "░", BarSkippedSelectedStyle
	default:
		return "░", BarSkippedSelectedStyle
	}
}

// getChildMarkerStyle returns the dimmed style for a child marker based on its conclusion
func getChildMarkerStyle(child *TreeItem) lipgloss.Style {
	switch child.Conclusion {
	case "success":
		return BarChildSuccessStyle
	case "failure":
		return BarChildFailureStyle
	default:
		return BarChildDefaultStyle
	}
}

// getChildMarkerStyleSelected returns the dimmed+selected style for a child marker
func getChildMarkerStyleSelected(child *TreeItem) lipgloss.Style {
	switch child.Conclusion {
	case "success":
		return BarChildSuccessSelectedStyle
	case "failure":
		return BarChildFailureSelectedStyle
	default:
		return BarChildDefaultSelectedStyle
	}
}

// childMarkerPos holds the timeline position and style for a single child marker
type childMarkerPos struct {
	pos   int
	style lipgloss.Style
}

// computeChildPositions calculates the timeline position for each immediate child
func computeChildPositions(children []*TreeItem, globalStart, globalEnd time.Time, width int, styleFn func(*TreeItem) lipgloss.Style) []childMarkerPos {
	totalDuration := globalEnd.Sub(globalStart)
	if totalDuration <= 0 || width <= 0 {
		return nil
	}

	var positions []childMarkerPos
	for _, child := range children {
		childStart := child.StartTime
		if childStart.IsZero() {
			continue
		}
		if childStart.Before(globalStart) {
			childStart = globalStart
		}
		if childStart.After(globalEnd) {
			childStart = globalEnd
		}

		pos := int(float64(childStart.Sub(globalStart)) / float64(totalDuration) * float64(width))
		if pos >= width {
			pos = width - 1
		}
		if pos < 0 {
			pos = 0
		}

		positions = append(positions, childMarkerPos{pos: pos, style: styleFn(child)})
	}
	return positions
}

// renderTimelineWithChildren builds a timeline bar with child markers overlaid.
// The buffer is filled with child markers first, then the parent bar overwrites on top.
// styleFn selects the appropriate child style variant (normal vs selected).
// If bgStyle is non-nil, empty space gets that background (for search-match rows).
// If selected is true, parent uses selected styles and padding gets selection bg.
func renderTimelineWithChildren(item TreeItem, globalStart, globalEnd time.Time, width int, url string, selected bool, bgStyle *lipgloss.Style) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		if selected {
			return SelectedBgStyle.Render(strings.Repeat(" ", width))
		}
		if bgStyle != nil {
			return bgStyle.Render(strings.Repeat(" ", width))
		}
		return strings.Repeat(" ", width)
	}

	totalDuration := globalEnd.Sub(globalStart)

	// Choose child style function based on mode
	childStyleFn := getChildMarkerStyle
	if selected {
		childStyleFn = getChildMarkerStyleSelected
	}

	// Compute child marker positions
	childPositions := computeChildPositions(item.Children, globalStart, globalEnd, width, childStyleFn)

	// Build buffer tracking what's at each position
	type cell struct {
		isChild bool
		style   lipgloss.Style
	}
	buf := make([]cell, width)

	// Place child markers
	for _, cp := range childPositions {
		buf[cp.pos] = cell{isChild: true, style: cp.style}
	}

	// Compute parent bar range
	parentStart := item.StartTime
	parentEnd := item.EndTime
	if parentStart.Before(globalStart) {
		parentStart = globalStart
	}
	if parentEnd.After(globalEnd) {
		parentEnd = globalEnd
	}

	isZeroDuration := parentEnd.Before(parentStart) || parentEnd.Equal(parentStart)

	var parentStartPos, parentBarLen int
	if isZeroDuration {
		startOffset := parentStart.Sub(globalStart)
		parentStartPos = int(float64(startOffset) / float64(totalDuration) * float64(width))
		if parentStartPos >= width {
			parentStartPos = width - 1
		}
		if parentStartPos < 0 {
			parentStartPos = 0
		}
		parentBarLen = 1
	} else {
		startOffset := parentStart.Sub(globalStart)
		endOffset := parentEnd.Sub(globalStart)
		parentStartPos = int(float64(startOffset) / float64(totalDuration) * float64(width))
		endPos := int(float64(endOffset) / float64(totalDuration) * float64(width))
		parentBarLen = endPos - parentStartPos
		if parentBarLen < 1 {
			parentBarLen = 1
		}
		if parentStartPos < 0 {
			parentStartPos = 0
		}
		if parentStartPos > width-1 {
			parentStartPos = width - 1
		}
		if parentStartPos+parentBarLen > width {
			parentBarLen = width - parentStartPos
		}
		if parentBarLen < 1 {
			parentBarLen = 1
		}
	}

	// Get parent bar character and style
	var barChar string
	var parentStyle lipgloss.Style
	if selected {
		barChar, parentStyle = getBarStyleSelected(item)
	} else {
		barChar, parentStyle = getBarStyle(item)
	}

	// For zero-duration non-marker, use | as indicator
	if isZeroDuration && item.ItemType != ItemTypeMarker {
		barChar = "|"
	}

	// Now build the output string by scanning the buffer and grouping runs
	var result strings.Builder
	i := 0
	for i < width {
		if i >= parentStartPos && i < parentStartPos+parentBarLen {
			// Parent bar region — collect consecutive parent chars
			end := parentStartPos + parentBarLen
			if end > width {
				end = width
			}
			count := end - i
			bar := strings.Repeat(barChar, count)
			styledBar := parentStyle.Render(bar)
			if !selected && bgStyle == nil {
				styledBar = timelineHyperlink(url, styledBar)
			}
			result.WriteString(styledBar)
			i = end
		} else if buf[i].isChild {
			// Child marker
			result.WriteString(buf[i].style.Render("·"))
			i++
		} else {
			// Empty space — collect consecutive spaces
			j := i
			for j < width && j != parentStartPos && !buf[j].isChild {
				if j >= parentStartPos && j < parentStartPos+parentBarLen {
					break
				}
				j++
			}
			spaces := strings.Repeat(" ", j-i)
			if selected {
				spaces = SelectedBgStyle.Render(spaces)
			} else if bgStyle != nil {
				spaces = bgStyle.Render(spaces)
			}
			result.WriteString(spaces)
			i = j
		}
	}

	return result.String()
}

// RenderTimelineBarWithChildren renders a timeline bar with dimmed child markers for collapsed items
func RenderTimelineBarWithChildren(item TreeItem, globalStart, globalEnd time.Time, width int, url string) string {
	return renderTimelineWithChildren(item, globalStart, globalEnd, width, url, false, nil)
}

// RenderTimelineBarWithChildrenSelected renders a timeline bar with child markers and selection background
func RenderTimelineBarWithChildrenSelected(item TreeItem, globalStart, globalEnd time.Time, width int, url string) string {
	return renderTimelineWithChildren(item, globalStart, globalEnd, width, url, true, nil)
}

// renderTimelineBarWithChildrenBg renders a timeline bar with child markers and a custom background
func renderTimelineBarWithChildrenBg(item TreeItem, globalStart, globalEnd time.Time, width int, url string, bg lipgloss.Style) string {
	return renderTimelineWithChildren(item, globalStart, globalEnd, width, url, false, &bg)
}

// RenderTimelineBarDimmed renders a timeline bar in gray for items after the logical end.
// It preserves the bar shape but uses BarSkippedStyle (gray) for all elements.
func RenderTimelineBarDimmed(item TreeItem, globalStart, globalEnd time.Time, width int) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return strings.Repeat(" ", width)
	}

	totalDuration := globalEnd.Sub(globalStart)

	itemStart := item.StartTime
	itemEnd := item.EndTime

	if itemStart.Before(globalStart) {
		itemStart = globalStart
	}
	if itemEnd.After(globalEnd) {
		itemEnd = globalEnd
	}

	// Handle 0-duration items
	isZeroDuration := itemEnd.Before(itemStart) || itemEnd.Equal(itemStart)
	if isZeroDuration {
		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))
		markerChar := "|"
		if item.ItemType == ItemTypeMarker {
			markerChar, _ = getBarStyle(item)
		}
		return renderMarker(markerChar, BarSkippedStyle, startPos, width, "", true)
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

	// Use the normal bar character but render in gray
	barChar, _ := getBarStyle(item)

	leftPad := strings.Repeat(" ", startPos)
	bar := strings.Repeat(barChar, barLength)
	rightPad := strings.Repeat(" ", width-startPos-barLength)

	return leftPad + BarSkippedStyle.Render(bar) + rightPad
}

// RenderTimelineBarDimmedSelected renders a dimmed timeline bar with selection background
func RenderTimelineBarDimmedSelected(item TreeItem, globalStart, globalEnd time.Time, width int) string {
	if globalEnd.Before(globalStart) || globalEnd.Equal(globalStart) || width <= 0 {
		return SelectedBgStyle.Render(strings.Repeat(" ", width))
	}

	totalDuration := globalEnd.Sub(globalStart)

	itemStart := item.StartTime
	itemEnd := item.EndTime

	if itemStart.Before(globalStart) {
		itemStart = globalStart
	}
	if itemEnd.After(globalEnd) {
		itemEnd = globalEnd
	}

	isZeroDuration := itemEnd.Before(itemStart) || itemEnd.Equal(itemStart)
	if isZeroDuration {
		startOffset := itemStart.Sub(globalStart)
		startPos := int(float64(startOffset) / float64(totalDuration) * float64(width))
		markerChar := "|"
		if item.ItemType == ItemTypeMarker {
			markerChar, _ = getBarStyle(item)
		}
		return renderMarker(markerChar, BarSkippedSelectedStyle, startPos, width, "", true)
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

	leftPad := SelectedBgStyle.Render(strings.Repeat(" ", startPos))
	bar := strings.Repeat(barChar, barLength)
	rightPad := SelectedBgStyle.Render(strings.Repeat(" ", width-startPos-barLength))

	return leftPad + BarSkippedSelectedStyle.Render(bar) + rightPad
}

// overlayLogicalEndLine replaces the character at visual column `col` in an
// ANSI-styled timeline string with a yellow "│". The replacement preserves
// total visible width. col must be in [0, width). If col < 0 the string is
// returned unchanged.
//
// When `selected` is true the marker gets a selection-bg behind it.
func overlayLogicalEndLine(timeline string, col, width int, selected bool) string {
	if col < 0 || col >= width {
		return timeline
	}

	// Walk the string tracking visible position. We split into three parts:
	// [before col] [char at col] [after col]
	// We rebuild: before + styled "│" + after
	type segment struct {
		start, end int // byte offsets in timeline
	}

	bytes := []byte(timeline)
	visPos := 0
	i := 0
	beforeEnd := 0   // byte offset where col starts
	afterStart := 0   // byte offset where col+1 starts
	found := false

	for i < len(bytes) && visPos <= col {
		if bytes[i] == '\x1b' {
			// Skip ANSI escape sequence
			j := i + 1
			if j < len(bytes) && bytes[j] == '[' {
				// CSI sequence: ESC [ ... final_byte
				j++
				for j < len(bytes) && bytes[j] < 0x40 {
					j++
				}
				if j < len(bytes) {
					j++ // skip final byte
				}
			} else if j < len(bytes) && bytes[j] == ']' {
				// OSC sequence: ESC ] ... ST (ST = ESC \ or BEL)
				j++
				for j < len(bytes) {
					if bytes[j] == '\x07' {
						j++
						break
					}
					if bytes[j] == '\x1b' && j+1 < len(bytes) && bytes[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
			}
			i = j
			continue
		}

		// Visible character — decode UTF-8 rune
		if visPos == col {
			beforeEnd = i
			// Skip this one rune
			r := 1
			if bytes[i] >= 0x80 {
				// Multi-byte UTF-8: find rune length
				for r < 4 && i+r < len(bytes) && (bytes[i+r]&0xC0) == 0x80 {
					r++
				}
			}
			// The character at col might be wider than 1, but we treat it as 1 column
			// since timeline positions are 1:1 with width
			afterStart = i + r
			// Continue past any trailing ANSI sequences that belong to this char
			j := afterStart
			for j < len(bytes) && bytes[j] == '\x1b' {
				k := j + 1
				if k < len(bytes) && bytes[k] == '[' {
					k++
					for k < len(bytes) && bytes[k] < 0x40 {
						k++
					}
					if k < len(bytes) {
						k++
					}
				} else if k < len(bytes) && bytes[k] == ']' {
					k++
					for k < len(bytes) {
						if bytes[k] == '\x07' {
							k++
							break
						}
						if bytes[k] == '\x1b' && k+1 < len(bytes) && bytes[k+1] == '\\' {
							k += 2
							break
						}
						k++
					}
				}
				j = k
			}
			afterStart = j
			found = true
			break
		}

		// Advance past this rune
		if bytes[i] < 0x80 {
			i++
		} else {
			r := 1
			for r < 4 && i+r < len(bytes) && (bytes[i+r]&0xC0) == 0x80 {
				r++
			}
			i += r
		}
		visPos++
	}

	if !found {
		return timeline
	}

	// Build the replacement
	markerStyle := LogicalEndBadgeStyle
	if selected {
		markerStyle = LogicalEndBadgeStyle.Background(ColorSelectionBg)
	}
	marker := markerStyle.Render("│")

	return string(bytes[:beforeEnd]) + marker + string(bytes[afterStart:])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
