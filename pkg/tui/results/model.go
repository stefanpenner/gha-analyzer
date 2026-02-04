package results

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

// ReloadResultMsg is sent when reload completes
type ReloadResultMsg struct {
	spans       []trace.ReadOnlySpan
	globalStart time.Time
	globalEnd   time.Time
	err         error
}

// LoadingProgressMsg updates loading progress display
type LoadingProgressMsg struct {
	Phase  string
	Detail string
	URL    string
}

// LoadingReporter reports loading progress
type LoadingReporter interface {
	SetPhase(phase string)
	SetDetail(detail string)
	SetURL(url string)
}

// Model represents the TUI state
type Model struct {
	roots         []*analyzer.TreeNode
	treeItems     []*TreeItem
	visibleItems  []TreeItem
	expandedState map[string]bool
	hiddenState   map[string]bool // items hidden from chart
	cursor        int
	selectionStart int // start of multi-selection range (-1 if no range)
	width         int
	height        int
	globalStart   time.Time
	globalEnd     time.Time
	chartStart    time.Time // calculated from non-hidden items
	chartEnd      time.Time // calculated from non-hidden items
	keys          KeyMap
	// Statistics (full dataset)
	summary     analyzer.Summary
	wallTimeMs  int64
	computeMs   int64
	stepCount   int
	// Displayed statistics (only visible items)
	displayedSummary   analyzer.Summary
	displayedWallTimeMs int64
	displayedComputeMs  int64
	displayedStepCount  int
	// Input URLs from CLI
	inputURLs []string
	// Modal state
	showDetailModal bool
	showHelpModal   bool
	modalItem       *TreeItem
	modalScroll     int
	// Reload state
	isLoading     bool
	reloadFunc    func(reporter LoadingReporter) ([]trace.ReadOnlySpan, time.Time, time.Time, error)
	spinner       spinner.Model
	loadingPhase  string
	loadingDetail string
	loadingURL    string
	progressCh    chan LoadingProgressMsg
	resultCh      chan ReloadResultMsg
	// Focus state
	isFocused           bool
	preFocusHiddenState map[string]bool
	// Spans for export
	spans []trace.ReadOnlySpan
	// Perfetto open function
	openPerfettoFunc func()
	// Mouse mode state
	mouseEnabled bool
}

// ReloadFunc is the function signature for reloading data
type ReloadFunc func(reporter LoadingReporter) ([]trace.ReadOnlySpan, time.Time, time.Time, error)

// OpenPerfettoFunc is the function signature for opening Perfetto
type OpenPerfettoFunc func()

// NewModel creates a new TUI model from OTel spans
func NewModel(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time, inputURLs []string, reloadFunc ReloadFunc, openPerfettoFunc OpenPerfettoFunc) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		expandedState:    make(map[string]bool),
		hiddenState:      make(map[string]bool),
		globalStart:      globalStart,
		globalEnd:        globalEnd,
		chartStart:       globalStart,
		chartEnd:         globalEnd,
		keys:             DefaultKeyMap(),
		width:            80,
		height:           24,
		inputURLs:        inputURLs,
		selectionStart:   -1, // no range selection initially
		reloadFunc:       reloadFunc,
		openPerfettoFunc: openPerfettoFunc,
		spinner:          s,
		spans:            spans,
	}

	// Calculate summary statistics
	m.summary = analyzer.CalculateSummary(spans)
	m.wallTimeMs = globalEnd.Sub(globalStart).Milliseconds()
	if m.wallTimeMs < 0 {
		m.wallTimeMs = 0
	}
	m.computeMs, m.stepCount = calculateComputeAndSteps(spans)

	// Initialize displayed stats to match full stats
	m.displayedSummary = m.summary
	m.displayedWallTimeMs = m.wallTimeMs
	m.displayedComputeMs = m.computeMs
	m.displayedStepCount = m.stepCount

	// Build tree from spans
	m.roots = analyzer.BuildTreeFromSpans(spans, globalStart, globalEnd)

	// Expand only workflows by default, jobs are collapsed
	m.expandAllToDepth(0)

	m.rebuildItems()
	return m
}

// calculateComputeAndSteps calculates total compute time and step count from spans
func calculateComputeAndSteps(spans []trace.ReadOnlySpan) (computeMs int64, stepCount int) {
	for _, s := range spans {
		var spanType string
		for _, a := range s.Attributes() {
			if string(a.Key) == "type" {
				spanType = a.Value.AsString()
				break
			}
		}
		if spanType == "job" {
			duration := s.EndTime().Sub(s.StartTime()).Milliseconds()
			if duration > 0 {
				computeMs += duration
			}
		} else if spanType == "step" {
			stepCount++
		}
	}
	return
}

// expandAllToDepth expands all items up to the given depth
func (m *Model) expandAllToDepth(maxDepth int) {
	var expand func(nodes []*analyzer.TreeNode, parentID string, depth int)
	expand = func(nodes []*analyzer.TreeNode, parentID string, depth int) {
		for i, node := range nodes {
			if depth <= maxDepth {
				id := makeNodeID(parentID, node.Name, i)
				m.expandedState[id] = true
				expand(node.Children, id, depth+1)
			}
		}
	}
	expand(m.roots, "", 0)
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ReloadResultMsg:
		m.isLoading = false
		m.progressCh = nil
		m.resultCh = nil
		m.loadingPhase = ""
		m.loadingDetail = ""
		m.loadingURL = ""
		if msg.err != nil {
			// TODO: show error in UI
			return m, nil
		}
		// Update model with new data
		m.globalStart = msg.globalStart
		m.globalEnd = msg.globalEnd
		m.chartStart = msg.globalStart
		m.chartEnd = msg.globalEnd
		m.summary = analyzer.CalculateSummary(msg.spans)
		m.wallTimeMs = msg.globalEnd.Sub(msg.globalStart).Milliseconds()
		if m.wallTimeMs < 0 {
			m.wallTimeMs = 0
		}
		m.computeMs, m.stepCount = calculateComputeAndSteps(msg.spans)
		m.roots = analyzer.BuildTreeFromSpans(msg.spans, msg.globalStart, msg.globalEnd)
		m.expandedState = make(map[string]bool)
		m.hiddenState = make(map[string]bool)
		m.expandAllToDepth(0)
		m.rebuildItems()
		m.cursor = 0
		m.selectionStart = -1
		return m, nil

	case spinner.TickMsg:
		if m.isLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case LoadingProgressMsg:
		if msg.Phase != "" {
			m.loadingPhase = msg.Phase
		}
		if msg.Detail != "" {
			m.loadingDetail = msg.Detail
		}
		if msg.URL != "" {
			m.loadingURL = msg.URL
		}
		// Continue listening for more progress updates
		return m, m.listenForProgress()

	case tea.KeyMsg:
		// Ignore keys while loading (except quit)
		if m.isLoading {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle help modal first
		if m.showHelpModal {
			switch msg.String() {
			case "esc", "enter", "?", "q":
				m.showHelpModal = false
				return m, nil
			}
			return m, nil
		}

		// Handle detail modal
		if m.showDetailModal {
			switch msg.String() {
			case "esc", "enter", "i", "q":
				m.showDetailModal = false
				m.modalItem = nil
				m.modalScroll = 0
				return m, nil
			case "up", "k":
				if m.modalScroll > 0 {
					m.modalScroll--
				}
				return m, nil
			case "down", "j":
				m.modalScroll++
				return m, nil
			case "left", "h":
				// Navigate to previous item
				if m.cursor > 0 {
					m.cursor--
					m.modalScroll = 0
					item := m.visibleItems[m.cursor]
					m.modalItem = &item
				}
				return m, nil
			case "right", "l":
				// Navigate to next item
				if m.cursor < len(m.visibleItems)-1 {
					m.cursor++
					m.modalScroll = 0
					item := m.visibleItems[m.cursor]
					m.modalItem = &item
				}
				return m, nil
			case "r":
				// Close modal and trigger reload
				m.showDetailModal = false
				m.modalItem = nil
				m.modalScroll = 0
				if m.reloadFunc != nil {
					m.isLoading = true
					return m, tea.Batch(m.spinner.Tick, m.doReload())
				}
				return m, nil
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Info):
			m.openDetailModal()
			return m, nil

		case key.Matches(msg, m.keys.Reload):
			if m.reloadFunc != nil {
				m.isLoading = true
				return m, tea.Batch(m.spinner.Tick, m.doReload())
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			m.selectionStart = -1 // clear selection
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keys.Down):
			m.selectionStart = -1 // clear selection
			if m.cursor < len(m.visibleItems)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keys.ShiftUp):
			// Start or extend selection upward
			if m.selectionStart == -1 {
				m.selectionStart = m.cursor
			}
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keys.ShiftDown):
			// Start or extend selection downward
			if m.selectionStart == -1 {
				m.selectionStart = m.cursor
			}
			if m.cursor < len(m.visibleItems)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keys.Left):
			m.selectionStart = -1 // clear selection
			m.collapseOrGoToParent()

		case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Enter):
			m.selectionStart = -1 // clear selection
			m.expandOrToggle()

		case key.Matches(msg, m.keys.Space):
			m.toggleChartVisibility()
			// Keep selection so user can toggle again or see what was selected

		case key.Matches(msg, m.keys.Open):
			m.openCurrentItem()

		case key.Matches(msg, m.keys.Focus):
			m.toggleFocus()

		case key.Matches(msg, m.keys.ExpandAll):
			m.expandAll()

		case key.Matches(msg, m.keys.CollapseAll):
			m.collapseAll()

		case key.Matches(msg, m.keys.Perfetto):
			if m.openPerfettoFunc != nil {
				m.openPerfettoFunc()
			}

		case key.Matches(msg, m.keys.Mouse):
			m.mouseEnabled = !m.mouseEnabled
			if m.mouseEnabled {
				return m, tea.EnableMouseCellMotion
			}
			return m, tea.DisableMouse

		case key.Matches(msg, m.keys.Help):
			m.showHelpModal = true
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.ClearScreen

	case tea.MouseMsg:
		// Ignore mouse while loading
		if m.isLoading {
			return m, nil
		}

		// Handle mouse in modal
		if m.showDetailModal {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.modalScroll > 0 {
					m.modalScroll--
				}
			case tea.MouseButtonWheelDown:
				m.modalScroll++
			case tea.MouseButtonLeft:
				if msg.Action == tea.MouseActionRelease {
					// Click outside modal area could close it (optional)
				}
			}
			return m, nil
		}

		// Handle mouse in main view
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Scroll up
			m.selectionStart = -1
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.MouseButtonWheelDown:
			// Scroll down
			m.selectionStart = -1
			if m.cursor < len(m.visibleItems)-1 {
				m.cursor++
			}
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionRelease {
				// Calculate which row was clicked
				headerLines := 8
				clickedRow := msg.Y - headerLines

				// Calculate scroll offset
				availableHeight := m.height - headerLines - 4
				if availableHeight < 1 {
					availableHeight = 10
				}

				startIdx := 0
				if len(m.visibleItems) > availableHeight {
					halfHeight := availableHeight / 2
					startIdx = m.cursor - halfHeight
					if startIdx < 0 {
						startIdx = 0
					}
					if startIdx+availableHeight > len(m.visibleItems) {
						startIdx = len(m.visibleItems) - availableHeight
						if startIdx < 0 {
							startIdx = 0
						}
					}
				}

				// Convert click position to item index
				itemIdx := startIdx + clickedRow
				if itemIdx >= 0 && itemIdx < len(m.visibleItems) {
					m.selectionStart = -1
					m.cursor = itemIdx
				}
			}
		}
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	// Enforce minimum dimensions to prevent crashes
	width := m.width
	height := m.height
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	// Show loading overlay if reloading
	if m.isLoading {
		loadingText := m.renderLoadingView()
		return placeModalCentered(ModalStyle.Render(loadingText), width, height)
	}

	var b strings.Builder

	// Calculate available height for items
	headerLines := 9 // header box (6 lines) + 1 newline + time axis + 1 blank line
	footerLines := 3 // blank line + help line + bottom border
	availableHeight := height - headerLines - footerLines
	if availableHeight < 1 {
		availableHeight = 10
	}

	// Determine if scrolling is needed
	totalItems := len(m.visibleItems)
	needsScroll := totalItems > availableHeight

	// Header (includes time range info)
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Time axis row (shows start time, duration, end time aligned with timeline)
	b.WriteString(m.renderTimeAxis())
	b.WriteString("\n")

	// Blank line between time axis and content (just outer borders, no middle separator)
	totalWidth := width - horizontalPad*2
	if totalWidth < 1 {
		totalWidth = 80
	}
	contentWidth := totalWidth - 2 // space between left and right borders
	blankLine := BorderStyle.Render("│") + strings.Repeat(" ", contentWidth) + BorderStyle.Render("│")
	b.WriteString(blankLine)
	b.WriteString("\n")

	// Determine scroll window
	startIdx := 0
	endIdx := totalItems

	if needsScroll {
		// Center cursor in view
		halfHeight := availableHeight / 2
		startIdx = m.cursor - halfHeight
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + availableHeight
		if endIdx > totalItems {
			endIdx = totalItems
			startIdx = endIdx - availableHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	// Calculate scrollbar dimensions (80% height, centered)
	trackHeight := availableHeight * 80 / 100
	if trackHeight < 3 {
		trackHeight = min(3, availableHeight)
	}
	trackTopPad := (availableHeight - trackHeight) / 2
	trackBottomPad := availableHeight - trackHeight - trackTopPad

	// Calculate thumb position within track
	thumbSize := 1
	thumbStart := 0
	if needsScroll && trackHeight > 0 {
		thumbSize = max(1, trackHeight*availableHeight/totalItems)
		if thumbSize > trackHeight {
			thumbSize = trackHeight
		}
		maxScroll := totalItems - availableHeight
		if maxScroll > 0 {
			thumbStart = startIdx * (trackHeight - thumbSize) / maxScroll
		}
	}
	thumbEnd := thumbStart + thumbSize

	// Scrollbar characters (use subtle separator color)
	scrollThumb := SeparatorStyle.Render("┃")
	scrollTrack := SeparatorStyle.Render("│")

	// Render visible items with scrollbar
	rowIdx := 0
	for i := startIdx; i < endIdx; i++ {
		item := m.visibleItems[i]
		isSelected := m.isInSelection(i)
		b.WriteString(m.renderItem(item, isSelected))

		// Add scrollbar character
		if needsScroll {
			trackIdx := rowIdx - trackTopPad
			if rowIdx < trackTopPad || rowIdx >= availableHeight-trackBottomPad {
				b.WriteString(" ")
			} else if trackIdx >= thumbStart && trackIdx < thumbEnd {
				b.WriteString(scrollThumb)
			} else {
				b.WriteString(scrollTrack)
			}
		}
		b.WriteString("\n")
		rowIdx++
	}

	// Pad if needed (with separator matching item rows)
	renderedItems := endIdx - startIdx
	for i := renderedItems; i < availableHeight; i++ {
		padTotalWidth := width - horizontalPad*2 // account for left/right padding
		if padTotalWidth < 1 {
			padTotalWidth = 80
		}
		// Match the structure: │ tree │ timeline │
		treeW := 55 // treeWidth constant
		availableW := padTotalWidth - 3
		timelineW := availableW - treeW
		if timelineW < 10 {
			timelineW = 10
		}
		b.WriteString(BorderStyle.Render("│") + strings.Repeat(" ", treeW) + SeparatorStyle.Render("│") + strings.Repeat(" ", timelineW) + BorderStyle.Render("│"))

		// Add scrollbar character for empty rows
		if needsScroll {
			trackIdx := rowIdx - trackTopPad
			if rowIdx < trackTopPad || rowIdx >= availableHeight-trackBottomPad {
				b.WriteString(" ")
			} else if trackIdx >= thumbStart && trackIdx < thumbEnd {
				b.WriteString(scrollThumb)
			} else {
				b.WriteString(scrollTrack)
			}
		}
		b.WriteString("\n")
		rowIdx++
	}

	// Blank line between content and footer (with borders)
	{
		footerTotalWidth := width - horizontalPad*2
		if footerTotalWidth < 1 {
			footerTotalWidth = 80
		}
		footerContentWidth := footerTotalWidth - 2
		blankLine := BorderStyle.Render("│") + strings.Repeat(" ", footerContentWidth) + BorderStyle.Render("│")
		b.WriteString(blankLine)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(m.renderFooter())

	// Overlay modal if showing
	if m.showHelpModal {
		modal := m.renderHelpModal()
		return placeModalCentered(modal, width, height)
	}

	if m.showDetailModal {
		modal, maxScroll := m.renderDetailModal(height-4, width-10)
		// Clamp scroll to valid range
		if m.modalScroll > maxScroll {
			m.modalScroll = maxScroll
		}
		return placeModalCentered(modal, width, height)
	}

	// Add horizontal padding to each line
	return addHorizontalPadding(b.String(), horizontalPad)
}

// addHorizontalPadding adds left padding to each line
func addHorizontalPadding(content string, pad int) string {
	lines := strings.Split(content, "\n")
	padStr := strings.Repeat(" ", pad)
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(padStr)
		result.WriteString(line)
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
}

// rebuildItems rebuilds the flattened item list based on expanded state
func (m *Model) rebuildItems() {
	m.treeItems = BuildTreeItems(m.roots, m.expandedState)
	m.visibleItems = FlattenVisibleItems(m.treeItems, m.expandedState)

	// Ensure cursor is valid
	if m.cursor >= len(m.visibleItems) {
		m.cursor = len(m.visibleItems) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// collapseOrGoToParent collapses the current item or moves to its parent
func (m *Model) collapseOrGoToParent() {
	if m.cursor >= len(m.visibleItems) {
		return
	}

	item := m.visibleItems[m.cursor]

	// If item is expanded, collapse it
	if item.HasChildren && m.expandedState[item.ID] {
		m.expandedState[item.ID] = false
		m.rebuildItems()
		return
	}

	// Otherwise, go to parent
	if item.ParentID != "" {
		for i, it := range m.visibleItems {
			if it.ID == item.ParentID {
				m.cursor = i
				break
			}
		}
	}
}

// expandOrToggle expands or toggles the current item
func (m *Model) expandOrToggle() {
	if m.cursor >= len(m.visibleItems) {
		return
	}

	item := m.visibleItems[m.cursor]
	if item.HasChildren {
		m.expandedState[item.ID] = !m.expandedState[item.ID]
		m.rebuildItems()
	}
}

// openCurrentItem opens the URL of the current item in a browser
func (m *Model) openCurrentItem() {
	if m.cursor >= len(m.visibleItems) {
		return
	}

	item := m.visibleItems[m.cursor]
	if item.URL != "" {
		_ = utils.OpenBrowser(item.URL)
	}
}

// renderLoadingView renders the loading progress display
func (m Model) renderLoadingView() string {
	var b strings.Builder

	// Header
	b.WriteString(ModalTitleStyle.Render("🚀 Reloading Data"))
	b.WriteString("\n\n")

	// URL being processed
	if m.loadingURL != "" {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(m.loadingURL)
		b.WriteString("\n")
	} else {
		b.WriteString(m.spinner.View())
		b.WriteString(" Loading...\n")
	}

	// Phase and detail
	if m.loadingPhase != "" {
		b.WriteString("  ↳ ")
		b.WriteString(m.loadingPhase)
		if m.loadingDetail != "" {
			b.WriteString(" (")
			b.WriteString(m.loadingDetail)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// openDetailModal opens the detail modal for the current item
func (m *Model) openDetailModal() {
	if m.cursor >= len(m.visibleItems) {
		return
	}

	item := m.visibleItems[m.cursor]
	m.modalItem = &item
	m.showDetailModal = true
	m.modalScroll = 0
}

// channelReporter implements LoadingReporter and sends updates via a channel
type channelReporter struct {
	ch chan<- LoadingProgressMsg
}

func (r *channelReporter) SetPhase(phase string) {
	select {
	case r.ch <- LoadingProgressMsg{Phase: phase}:
	default:
	}
}

func (r *channelReporter) SetDetail(detail string) {
	select {
	case r.ch <- LoadingProgressMsg{Detail: detail}:
	default:
	}
}

func (r *channelReporter) SetURL(url string) {
	select {
	case r.ch <- LoadingProgressMsg{URL: url}:
	default:
	}
}

// doReload returns a command that performs the reload with progress updates
func (m *Model) doReload() tea.Cmd {
	// Store channels in model for listenForProgress to access
	m.progressCh = make(chan LoadingProgressMsg, 10)
	m.resultCh = make(chan ReloadResultMsg, 1)

	reporter := &channelReporter{ch: m.progressCh}
	progressCh := m.progressCh
	resultCh := m.resultCh

	// Start the reload in a goroutine
	go func() {
		defer close(progressCh)
		spans, start, end, err := m.reloadFunc(reporter)
		resultCh <- ReloadResultMsg{
			spans:       spans,
			globalStart: start,
			globalEnd:   end,
			err:         err,
		}
	}()

	// Return a command that listens for progress and result
	return m.listenForProgress()
}

// listenForProgress returns a command that listens for progress updates or results
func (m *Model) listenForProgress() tea.Cmd {
	progressCh := m.progressCh
	resultCh := m.resultCh

	if progressCh == nil || resultCh == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case progress, ok := <-progressCh:
			if ok {
				return progress
			}
			// Channel closed, wait for result
			return <-resultCh
		case result := <-resultCh:
			return result
		}
	}
}

// expandAll expands all items
func (m *Model) expandAll() {
	var expandNodes func(nodes []*TreeItem)
	expandNodes = func(nodes []*TreeItem) {
		for _, item := range nodes {
			if item.HasChildren {
				m.expandedState[item.ID] = true
			}
			expandNodes(item.Children)
		}
	}
	expandNodes(m.treeItems)
	m.rebuildItems()
}

// collapseAll collapses all items
func (m *Model) collapseAll() {
	for id := range m.expandedState {
		m.expandedState[id] = false
	}
	m.rebuildItems()
}

// toggleFocus focuses on the current selection, hiding everything else
func (m *Model) toggleFocus() {
	if m.isFocused {
		// Unfocus: restore the previous hidden state
		m.hiddenState = m.preFocusHiddenState
		m.preFocusHiddenState = nil
		m.isFocused = false
	} else {
		// Focus: save current hidden state and hide everything except selection
		m.preFocusHiddenState = make(map[string]bool)
		for k, v := range m.hiddenState {
			m.preFocusHiddenState[k] = v
		}

		// Get selected items
		start, end := m.getSelectionRange()
		selectedIDs := make(map[string]bool)
		for i := start; i <= end && i < len(m.visibleItems); i++ {
			item := m.visibleItems[i]
			selectedIDs[item.ID] = true
			// Also include all ancestors (parents) to keep context
			m.collectAncestorIDs(item.ParentID, selectedIDs)
			// Also include all descendants
			m.collectDescendantIDs(item.ID, selectedIDs)
		}

		// Hide everything except selected items and their ancestors/descendants
		var hideAll func(items []*TreeItem)
		hideAll = func(items []*TreeItem) {
			for _, item := range items {
				if !selectedIDs[item.ID] {
					m.hiddenState[item.ID] = true
				} else {
					m.hiddenState[item.ID] = false
				}
				hideAll(item.Children)
			}
		}
		hideAll(m.treeItems)
		m.isFocused = true
	}
	m.recalculateChartBounds()
}

// collectAncestorIDs adds all ancestor IDs to the set
func (m *Model) collectAncestorIDs(parentID string, ids map[string]bool) {
	if parentID == "" {
		return
	}
	ids[parentID] = true
	// Find the parent item to get its parent
	for _, item := range m.visibleItems {
		if item.ID == parentID {
			m.collectAncestorIDs(item.ParentID, ids)
			return
		}
	}
}

// collectDescendantIDs adds all descendant IDs to the set
func (m *Model) collectDescendantIDs(parentID string, ids map[string]bool) {
	var collect func(items []*TreeItem)
	collect = func(items []*TreeItem) {
		for _, item := range items {
			if item.ParentID == parentID || ids[item.ParentID] {
				ids[item.ID] = true
			}
			collect(item.Children)
		}
	}
	collect(m.treeItems)
}

// getSelectionRange returns the start and end indices of the current selection
func (m *Model) getSelectionRange() (start, end int) {
	if m.selectionStart == -1 {
		return m.cursor, m.cursor
	}
	if m.selectionStart < m.cursor {
		return m.selectionStart, m.cursor
	}
	return m.cursor, m.selectionStart
}

// isInSelection returns true if the given index is within the current selection
func (m *Model) isInSelection(idx int) bool {
	start, end := m.getSelectionRange()
	return idx >= start && idx <= end
}

// toggleChartVisibility toggles visibility for all items in the selection range
func (m *Model) toggleChartVisibility() {
	start, end := m.getSelectionRange()
	if start >= len(m.visibleItems) {
		return
	}
	if end >= len(m.visibleItems) {
		end = len(m.visibleItems) - 1
	}

	// Determine target state from first item in selection
	firstItem := m.visibleItems[start]
	targetHidden := !m.hiddenState[firstItem.ID]

	// Toggle all items in selection range
	for i := start; i <= end; i++ {
		item := m.visibleItems[i]
		m.hiddenState[item.ID] = targetHidden
		// Also toggle all children of this item
		m.toggleChildrenVisibility(item.ID, targetHidden)
	}

	// Recalculate chart bounds
	m.recalculateChartBounds()
}

// toggleChildrenVisibility recursively sets visibility for all descendants in the tree
func (m *Model) toggleChildrenVisibility(parentID string, hidden bool) {
	// Find the item in the tree and toggle all its descendants
	var findAndToggle func(items []*TreeItem) bool
	findAndToggle = func(items []*TreeItem) bool {
		for _, item := range items {
			if item.ID == parentID {
				// Found the parent, toggle all its children
				m.toggleDescendants(item.Children, hidden)
				return true
			}
			if findAndToggle(item.Children) {
				return true
			}
		}
		return false
	}
	findAndToggle(m.treeItems)
}

// toggleDescendants recursively sets visibility for items and all their descendants
func (m *Model) toggleDescendants(items []*TreeItem, hidden bool) {
	for _, item := range items {
		m.hiddenState[item.ID] = hidden
		m.toggleDescendants(item.Children, hidden)
	}
}

// recalculateChartBounds recalculates the chart time window and stats based on visible items
func (m *Model) recalculateChartBounds() {
	var earliest, latest time.Time
	var totalRuns, successfulRuns int
	var totalJobs, failedJobs int
	var stepCount int
	var computeMs int64
	workflowsSeen := make(map[string]bool)

	var checkItems func(items []*TreeItem)
	checkItems = func(items []*TreeItem) {
		for _, item := range items {
			if m.hiddenState[item.ID] {
				continue
			}

			// Time bounds
			if !item.StartTime.IsZero() {
				if earliest.IsZero() || item.StartTime.Before(earliest) {
					earliest = item.StartTime
				}
			}
			if !item.EndTime.IsZero() {
				if latest.IsZero() || item.EndTime.After(latest) {
					latest = item.EndTime
				}
			}

			// Stats by item type
			switch item.ItemType {
			case ItemTypeWorkflow:
				if !workflowsSeen[item.Name] {
					workflowsSeen[item.Name] = true
					totalRuns++
					if item.Conclusion == "success" {
						successfulRuns++
					}
				}
			case ItemTypeJob:
				totalJobs++
				if item.Conclusion == "failure" {
					failedJobs++
				}
				// Compute time is sum of job durations
				if !item.StartTime.IsZero() && !item.EndTime.IsZero() {
					duration := item.EndTime.Sub(item.StartTime).Milliseconds()
					if duration > 0 {
						computeMs += duration
					}
				}
			case ItemTypeStep:
				stepCount++
			}

			checkItems(item.Children)
		}
	}
	checkItems(m.treeItems)

	// If all items are hidden, use global bounds and full stats
	if earliest.IsZero() {
		earliest = m.globalStart
		m.displayedSummary = m.summary
		m.displayedWallTimeMs = m.wallTimeMs
		m.displayedComputeMs = m.computeMs
		m.displayedStepCount = m.stepCount
	} else {
		// Update displayed stats
		m.displayedSummary = analyzer.Summary{
			TotalRuns:      totalRuns,
			SuccessfulRuns: successfulRuns,
			TotalJobs:      totalJobs,
			FailedJobs:     failedJobs,
			MaxConcurrency: m.summary.MaxConcurrency, // Keep original concurrency
		}
		m.displayedStepCount = stepCount
		m.displayedComputeMs = computeMs
	}
	if latest.IsZero() {
		latest = m.globalEnd
	}

	m.chartStart = earliest
	m.chartEnd = latest

	// Wall time is chart duration
	m.displayedWallTimeMs = latest.Sub(earliest).Milliseconds()
	if m.displayedWallTimeMs < 0 {
		m.displayedWallTimeMs = 0
	}
}

// IsHidden returns whether an item is hidden from the chart
func (m *Model) IsHidden(id string) bool {
	return m.hiddenState[id]
}

// Run starts the TUI
func Run(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time, inputURLs []string, reloadFunc ReloadFunc, openPerfettoFunc OpenPerfettoFunc) error {
	m := NewModel(spans, globalStart, globalEnd, inputURLs, reloadFunc, openPerfettoFunc)
	// Mouse mode disabled by default to allow OSC 8 hyperlinks to work
	// Press 'm' to toggle mouse mode for scrolling
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("tea.Program.Run failed: %w", err)
	}
	_ = finalModel
	return nil
}
