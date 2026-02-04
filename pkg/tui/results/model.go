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
	// Statistics
	summary     analyzer.Summary
	wallTimeMs  int64
	computeMs   int64
	stepCount   int
	// Input URLs from CLI
	inputURLs []string
	// Modal state
	showDetailModal bool
	modalItem       *TreeItem
	modalScroll     int
	// Reload state
	isLoading  bool
	reloadFunc func() ([]trace.ReadOnlySpan, time.Time, time.Time, error)
	spinner    spinner.Model
	// Focus state
	isFocused          bool
	preFocusHiddenState map[string]bool
}

// ReloadFunc is the function signature for reloading data
type ReloadFunc func() ([]trace.ReadOnlySpan, time.Time, time.Time, error)

// NewModel creates a new TUI model from OTel spans
func NewModel(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time, inputURLs []string, reloadFunc ReloadFunc) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		expandedState:  make(map[string]bool),
		hiddenState:    make(map[string]bool),
		globalStart:    globalStart,
		globalEnd:      globalEnd,
		chartStart:     globalStart,
		chartEnd:       globalEnd,
		keys:           DefaultKeyMap(),
		width:          80,
		height:         24,
		inputURLs:      inputURLs,
		selectionStart: -1, // no range selection initially
		reloadFunc:     reloadFunc,
		spinner:        s,
	}

	// Calculate summary statistics
	m.summary = analyzer.CalculateSummary(spans)
	m.wallTimeMs = globalEnd.Sub(globalStart).Milliseconds()
	m.computeMs, m.stepCount = calculateComputeAndSteps(spans)

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

	case tea.KeyMsg:
		// Ignore keys while loading (except quit)
		if m.isLoading {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle modal state first
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.ClearScreen
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	// Show loading overlay if reloading
	if m.isLoading {
		loadingText := m.spinner.View() + " Reloading data..."
		return placeModalCentered(ModalStyle.Render(loadingText), m.width, m.height)
	}

	var b strings.Builder

	// Header (includes time range info)
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Calculate available height for items
	headerLines := 8 // header box (up to 7 lines + margin)
	footerLines := 3
	availableHeight := m.height - headerLines - footerLines
	if availableHeight < 1 {
		availableHeight = 10
	}

	// Determine scroll window
	startIdx := 0
	endIdx := len(m.visibleItems)
	if len(m.visibleItems) > availableHeight {
		// Center cursor in view
		halfHeight := availableHeight / 2
		startIdx = m.cursor - halfHeight
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + availableHeight
		if endIdx > len(m.visibleItems) {
			endIdx = len(m.visibleItems)
			startIdx = endIdx - availableHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	// Render visible items
	for i := startIdx; i < endIdx; i++ {
		item := m.visibleItems[i]
		isSelected := m.isInSelection(i)
		b.WriteString(m.renderItem(item, isSelected))
		b.WriteString("\n")
	}

	// Pad if needed (with separator matching item rows)
	renderedItems := endIdx - startIdx
	for i := renderedItems; i < availableHeight; i++ {
		totalWidth := m.width
		if totalWidth < 1 {
			totalWidth = 80
		}
		// Match the structure: │ tree │ timeline │
		treeW := 55 // treeWidth constant
		availableW := totalWidth - 3
		timelineW := availableW - treeW
		if timelineW < 10 {
			timelineW = 10
		}
		b.WriteString(BorderStyle.Render("│") + strings.Repeat(" ", treeW) + BorderStyle.Render("│") + strings.Repeat(" ", timelineW) + BorderStyle.Render("│") + "\n")
	}

	// Footer
	b.WriteString(m.renderFooter())

	// Overlay modal if showing
	if m.showDetailModal {
		modal, maxScroll := m.renderDetailModal(m.height-4, m.width-10)
		// Clamp scroll to valid range
		if m.modalScroll > maxScroll {
			m.modalScroll = maxScroll
		}
		return placeModalCentered(modal, m.width, m.height)
	}

	return b.String()
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

// doReload returns a command that performs the reload
func (m *Model) doReload() tea.Cmd {
	return func() tea.Msg {
		spans, start, end, err := m.reloadFunc()
		return ReloadResultMsg{
			spans:       spans,
			globalStart: start,
			globalEnd:   end,
			err:         err,
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

// recalculateChartBounds recalculates the chart time window based on visible items
func (m *Model) recalculateChartBounds() {
	var earliest, latest time.Time

	var checkItems func(items []*TreeItem)
	checkItems = func(items []*TreeItem) {
		for _, item := range items {
			if m.hiddenState[item.ID] {
				continue
			}
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
			checkItems(item.Children)
		}
	}
	checkItems(m.treeItems)

	// If all items are hidden, use global bounds
	if earliest.IsZero() {
		earliest = m.globalStart
	}
	if latest.IsZero() {
		latest = m.globalEnd
	}

	m.chartStart = earliest
	m.chartEnd = latest
}

// IsHidden returns whether an item is hidden from the chart
func (m *Model) IsHidden(id string) bool {
	return m.hiddenState[id]
}

// Run starts the TUI
func Run(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time, inputURLs []string, reloadFunc ReloadFunc) error {
	m := NewModel(spans, globalStart, globalEnd, inputURLs, reloadFunc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("tea.Program.Run failed: %w", err)
	}
	_ = finalModel
	return nil
}
