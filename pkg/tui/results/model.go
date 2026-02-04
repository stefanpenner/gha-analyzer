package results

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Model represents the TUI state
type Model struct {
	roots         []*analyzer.TreeNode
	treeItems     []*TreeItem
	visibleItems  []TreeItem
	expandedState map[string]bool
	cursor        int
	width         int
	height        int
	globalStart   time.Time
	globalEnd     time.Time
	keys          KeyMap
	// Statistics
	summary     analyzer.Summary
	wallTimeMs  int64
	computeMs   int64
	stepCount   int
	// Source URL (PR or commit)
	sourceURL   string
	sourceName  string
}

// NewModel creates a new TUI model from OTel spans
func NewModel(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time) Model {
	m := Model{
		expandedState: make(map[string]bool),
		globalStart:   globalStart,
		globalEnd:     globalEnd,
		keys:          DefaultKeyMap(),
		width:         80,
		height:        24,
	}

	// Calculate summary statistics
	m.summary = analyzer.CalculateSummary(spans)
	m.wallTimeMs = globalEnd.Sub(globalStart).Milliseconds()
	m.computeMs, m.stepCount = calculateComputeAndSteps(spans)
	m.sourceURL, m.sourceName = extractSourceInfo(spans)

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

// extractSourceInfo extracts the source URL and name from spans (e.g., PR or commit URL)
func extractSourceInfo(spans []trace.ReadOnlySpan) (url, name string) {
	// Look for the first workflow span with a source URL
	for _, s := range spans {
		attrs := make(map[string]string)
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.AsString()
		}
		if attrs["type"] == "workflow" {
			if u := attrs["github.source_url"]; u != "" {
				return u, attrs["github.source_name"]
			}
			// Fallback to workflow URL
			if u := attrs["github.url"]; u != "" {
				return u, s.Name()
			}
		}
	}
	return "", ""
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
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.visibleItems)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keys.Left):
			m.collapseOrGoToParent()

		case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Enter), key.Matches(msg, m.keys.Space):
			m.expandOrToggle()

		case key.Matches(msg, m.keys.Open):
			m.openCurrentItem()

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
	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Time header
	timeHeader := m.renderTimeHeader()
	if timeHeader != "" {
		b.WriteString(timeHeader)
		b.WriteString("\n")
	}

	// Calculate available height for items
	headerLines := 7 // header box (up to 6 lines + margin)
	timeHeaderLines := 2
	footerLines := 3
	availableHeight := m.height - headerLines - timeHeaderLines - footerLines
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
		isSelected := i == m.cursor
		b.WriteString(m.renderItem(item, isSelected))
		b.WriteString("\n")
	}

	// Pad if needed
	renderedItems := endIdx - startIdx
	for i := renderedItems; i < availableHeight; i++ {
		totalWidth := m.width
		if totalWidth < 1 {
			totalWidth = 80
		}
		b.WriteString("│" + strings.Repeat(" ", max(0, totalWidth-2)) + "│\n")
	}

	// Footer
	b.WriteString(m.renderFooter())

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

// Run starts the TUI
func Run(spans []trace.ReadOnlySpan, globalStart, globalEnd time.Time) error {
	m := NewModel(spans, globalStart, globalEnd)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("tea.Program.Run failed: %w", err)
	}
	_ = finalModel
	return nil
}
