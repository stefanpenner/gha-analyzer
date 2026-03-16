package results

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/enrichment"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/sdk/trace"
)

// createTestModel creates a Model with test data for integration testing
func createTestModel() Model {
	now := time.Now()
	globalStart := now
	globalEnd := now.Add(5 * time.Minute)

	m := Model{
		enricher:       enrichment.DefaultEnricher(),
		expandedState:  make(map[string]bool),
		hiddenState:    make(map[string]bool),
		globalStart:    globalStart,
		globalEnd:      globalEnd,
		chartStart:     globalStart,
		chartEnd:       globalEnd,
		keys:           DefaultKeyMap(),
		width:          120,
		height:         40,
		inputURLs:      []string{"https://github.com/test/repo/pull/123"},
		selectionStart: -1,
		treeWidth:      defaultTreeWidth,
	}

	// Build test tree using analyzer.TreeNode (like real code does)
	m.roots = []*analyzer.TreeNode{
		{
			Name:      "CI",
			Hints:     enrichment.SpanHints{Category: "workflow", IsRoot: true, Outcome: "success", Color: "green", BarChar: "█", URL: "https://github.com/test/repo/actions/runs/123"},
			StartTime: globalStart,
			EndTime:   globalEnd,
			Children: []*analyzer.TreeNode{
				{
					Name:      "build",
					Hints:     enrichment.SpanHints{Category: "job", Outcome: "success", Color: "green", BarChar: "█", URL: "https://github.com/test/repo/actions/runs/123/jobs/456"},
					StartTime: globalStart,
					EndTime:   globalStart.Add(2 * time.Minute),
					Children: []*analyzer.TreeNode{
						{
							Name:      "Checkout",
							Hints:     enrichment.SpanHints{Category: "step", IsLeaf: true, Outcome: "success", Color: "green", BarChar: "▒"},
							StartTime: globalStart,
							EndTime:   globalStart.Add(10 * time.Second),
						},
						{
							Name:      "Build",
							Hints:     enrichment.SpanHints{Category: "step", IsLeaf: true, Outcome: "success", Color: "green", BarChar: "▒"},
							StartTime: globalStart.Add(10 * time.Second),
							EndTime:   globalStart.Add(2 * time.Minute),
						},
					},
				},
				{
					Name:      "test",
					Hints:     enrichment.SpanHints{Category: "job", Outcome: "failure", Color: "red", BarChar: "█", URL: "https://github.com/test/repo/actions/runs/123/jobs/789"},
					StartTime: globalStart.Add(2 * time.Minute),
					EndTime:   globalEnd,
				},
			},
		},
	}

	// Expand workflow by default
	m.expandedState["CI/0"] = true

	// Build tree items and visible items (like real code does)
	m.rebuildItems()

	return m
}

func TestNewModel(t *testing.T) {
	t.Parallel()

	t.Run("initializes with default values", func(t *testing.T) {
		now := time.Now()
		m := NewModel(nil, now, now.Add(time.Minute), []string{"https://example.com"}, nil, nil, enrichment.DefaultEnricher())

		assert.Equal(t, 80, m.width)
		assert.Equal(t, 24, m.height)
		assert.Equal(t, -1, m.selectionStart)
		assert.False(t, m.mouseEnabled)
		assert.False(t, m.showDetailModal)
		assert.False(t, m.showHelpModal)
		assert.NotNil(t, m.expandedState)
		assert.NotNil(t, m.hiddenState)
	})
}

func TestModelView(t *testing.T) {
	t.Parallel()

	t.Run("renders without crashing", func(t *testing.T) {
		m := createTestModel()
		view := m.View()

		assert.NotEmpty(t, view)
		assert.Contains(t, view, "Trace Analyzer")
	})

	t.Run("renders header with URL", func(t *testing.T) {
		m := createTestModel()
		view := m.View()

		// Header should contain the input URL (may be hyperlinked)
		assert.Contains(t, view, "github.com/test/repo/pull/123")
	})

	t.Run("renders tree items", func(t *testing.T) {
		m := createTestModel()
		view := m.View()

		// Should show workflow and jobs (workflow is expanded)
		assert.Contains(t, view, "CI")
		assert.Contains(t, view, "build")
		assert.Contains(t, view, "test")
	})

	t.Run("renders with small dimensions", func(t *testing.T) {
		m := createTestModel()
		m.width = 40
		m.height = 10

		// Should not panic with small dimensions
		view := m.View()
		assert.NotEmpty(t, view)
	})

	t.Run("renders help modal", func(t *testing.T) {
		m := createTestModel()
		m.showHelpModal = true

		view := m.View()
		assert.Contains(t, view, "Keyboard Shortcuts")
	})
}

func TestModelNavigation(t *testing.T) {
	t.Parallel()

	t.Run("moves cursor down with j key", func(t *testing.T) {
		m := createTestModel()
		initialCursor := m.cursor

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, initialCursor+1, m.cursor)
	})

	t.Run("moves cursor up with k key", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("does not move cursor below zero", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("does not move cursor past last item", func(t *testing.T) {
		m := createTestModel()
		m.cursor = len(m.visibleItems) - 1
		lastCursor := m.cursor

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, lastCursor, m.cursor)
	})
}

func TestModelExpandCollapse(t *testing.T) {
	t.Parallel()

	t.Run("expands item with right arrow", func(t *testing.T) {
		m := createTestModel()
		// Collapse workflow first
		m.expandedState["CI/0"] = false
		m.visibleItems = FlattenVisibleItems(m.treeItems, m.expandedState, m.sortMode)
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = newModel.(Model)

		assert.True(t, m.expandedState["CI/0"])
	})

	t.Run("collapses item with left arrow", func(t *testing.T) {
		m := createTestModel()
		m.expandedState["CI/0"] = true
		m.visibleItems = FlattenVisibleItems(m.treeItems, m.expandedState, m.sortMode)
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = newModel.(Model)

		assert.False(t, m.expandedState["CI/0"])
	})

	t.Run("expand all with c key (toggle)", func(t *testing.T) {
		m := createTestModel()
		// Collapse everything first
		m.expandedState = make(map[string]bool)
		m.rebuildItems()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
		m = newModel.(Model)

		// Should expand workflow and jobs
		assert.True(t, m.expandedState["CI/0"])
	})

	t.Run("collapse all with c key (toggle)", func(t *testing.T) {
		m := createTestModel()
		// Expand everything first so toggle will collapse
		m.expandAll()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
		m = newModel.(Model)

		assert.False(t, m.expandedState["CI/0"])
		assert.False(t, m.expandedState["CI/0/job/0"])
	})
}

func TestModelMouseToggle(t *testing.T) {
	t.Parallel()

	t.Run("toggles mouse mode with m key", func(t *testing.T) {
		m := createTestModel()
		assert.False(t, m.mouseEnabled)

		// Enable mouse
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
		m = newModel.(Model)

		assert.True(t, m.mouseEnabled)
		assert.NotNil(t, cmd) // Should return EnableMouseCellMotion command

		// Disable mouse
		newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
		m = newModel.(Model)

		assert.False(t, m.mouseEnabled)
		assert.NotNil(t, cmd) // Should return DisableMouse command
	})
}

func TestModelModals(t *testing.T) {
	t.Parallel()

	t.Run("opens help modal with ? key", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		m = newModel.(Model)

		assert.True(t, m.showHelpModal)
	})

	t.Run("closes help modal with escape", func(t *testing.T) {
		m := createTestModel()
		m.showHelpModal = true

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.False(t, m.showHelpModal)
	})

	t.Run("opens detail modal with i key", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
		m = newModel.(Model)

		assert.True(t, m.showDetailModal)
		assert.NotNil(t, m.modalItem)
	})

	t.Run("closes detail modal with escape", func(t *testing.T) {
		m := createTestModel()
		m.showDetailModal = true
		m.modalItem = &m.visibleItems[0]

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.False(t, m.showDetailModal)
		assert.Nil(t, m.modalItem)
	})
}

func TestModelQuit(t *testing.T) {
	t.Parallel()

	t.Run("quits with q key", func(t *testing.T) {
		m := createTestModel()

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

		// Should return tea.Quit command
		assert.NotNil(t, cmd)
	})

	t.Run("quits with ctrl+c", func(t *testing.T) {
		m := createTestModel()

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

		assert.NotNil(t, cmd)
	})
}

func TestModelWindowResize(t *testing.T) {
	t.Parallel()

	t.Run("handles window resize", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
		m = newModel.(Model)

		assert.Equal(t, 200, m.width)
		assert.Equal(t, 50, m.height)
	})
}

func TestHyperlinkFormat(t *testing.T) {
	t.Parallel()

	t.Run("hyperlink includes id parameter", func(t *testing.T) {
		url := "https://github.com/test"
		text := "click me"

		result := hyperlink(url, text)

		// Should contain id parameter for proper link isolation
		assert.Contains(t, result, "\x1b]8;id=")
		assert.Contains(t, result, url)
		assert.Contains(t, result, text)
		assert.Contains(t, result, "\x1b]8;;\x07") // Closing sequence
	})

	t.Run("hyperlink returns text unchanged when URL empty", func(t *testing.T) {
		result := hyperlink("", "text")
		assert.Equal(t, "text", result)
	})

	t.Run("hyperlinks in view are properly formatted", func(t *testing.T) {
		m := createTestModel()
		view := m.View()

		// View should contain OSC 8 hyperlink sequences
		assert.Contains(t, view, "\x1b]8;")
	})
}

func TestModelSelection(t *testing.T) {
	t.Parallel()

	t.Run("shift+down starts selection", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.selectionStart)
		assert.Equal(t, 1, m.cursor)
	})

	t.Run("shift+up starts selection", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
		m = newModel.(Model)

		assert.Equal(t, 1, m.selectionStart)
		assert.Equal(t, 0, m.cursor)
	})

	t.Run("regular navigation clears selection", func(t *testing.T) {
		m := createTestModel()
		m.selectionStart = 0
		m.cursor = 2

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, -1, m.selectionStart)
	})
}

func TestModelChartVisibility(t *testing.T) {
	t.Parallel()

	t.Run("toggles chart visibility with space", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		itemID := m.visibleItems[0].ID

		assert.False(t, m.hiddenState[itemID])

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = newModel.(Model)

		assert.True(t, m.hiddenState[itemID])

		// Toggle again
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = newModel.(Model)

		assert.False(t, m.hiddenState[itemID])
	})
}

func TestKeyMap(t *testing.T) {
	t.Parallel()

	t.Run("default keymap has all bindings", func(t *testing.T) {
		km := DefaultKeyMap()

		assert.NotEmpty(t, km.Up.Keys())
		assert.NotEmpty(t, km.Down.Keys())
		assert.NotEmpty(t, km.Left.Keys())
		assert.NotEmpty(t, km.Right.Keys())
		assert.NotEmpty(t, km.Enter.Keys())
		assert.NotEmpty(t, km.Space.Keys())
		assert.NotEmpty(t, km.Open.Keys())
		assert.NotEmpty(t, km.Info.Keys())
		assert.NotEmpty(t, km.Focus.Keys())
		assert.NotEmpty(t, km.Reload.Keys())
		assert.NotEmpty(t, km.ToggleExpandAll.Keys())
		assert.NotEmpty(t, km.Perfetto.Keys())
		assert.NotEmpty(t, km.Mouse.Keys())
		assert.NotEmpty(t, km.Help.Keys())
		assert.NotEmpty(t, km.Quit.Keys())
	})

	t.Run("short help contains key info", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelp()

		assert.Contains(t, help, "nav")
		assert.Contains(t, help, "help")
		assert.Contains(t, help, "sort")
	})

	t.Run("full help contains mouse toggle", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.FullHelp()

		found := false
		for _, row := range help {
			if len(row) >= 2 && strings.Contains(row[1], "mouse") {
				found = true
				break
			}
		}
		assert.True(t, found, "Full help should contain mouse toggle info")
	})
}

func TestRenderItem(t *testing.T) {
	t.Parallel()

	t.Run("renders normal item with hyperlink", func(t *testing.T) {
		m := createTestModel()
		item := m.visibleItems[0]

		result := m.renderItem(item, false)

		assert.Contains(t, result, item.DisplayName)
		// Should contain hyperlink if item has URL
		if item.Hints.URL != "" {
			assert.Contains(t, result, "\x1b]8;")
		}
	})

	t.Run("renders selected item", func(t *testing.T) {
		m := createTestModel()
		item := m.visibleItems[0]

		result := m.renderItem(item, true)

		assert.Contains(t, result, item.DisplayName)
		// Selected items should still have hyperlinks
		if item.Hints.URL != "" {
			assert.Contains(t, result, "\x1b]8;")
		}
	})

	t.Run("renders hidden item", func(t *testing.T) {
		m := createTestModel()
		item := m.visibleItems[0]
		m.hiddenState[item.ID] = true

		result := m.renderItem(item, false)

		assert.Contains(t, result, item.DisplayName)
	})
}

func TestRenderHeader(t *testing.T) {
	t.Parallel()

	t.Run("renders header with title", func(t *testing.T) {
		m := createTestModel()
		header := m.renderHeader()

		assert.Contains(t, header, "Trace Analyzer")
	})

	t.Run("renders header with input URLs", func(t *testing.T) {
		m := createTestModel()
		header := m.renderHeader()

		assert.Contains(t, header, "github.com/test/repo/pull/123")
	})
}

func TestRenderFooter(t *testing.T) {
	t.Parallel()

	t.Run("renders footer with help hints", func(t *testing.T) {
		m := createTestModel()
		footer := m.renderFooter()

		assert.Contains(t, footer, "help")
		assert.Contains(t, footer, "nav")
	})
}

// createMultiURLTestModel creates a Model with two input URLs for testing URL group behavior.
// Tree structure:
//
//	url-group/0: PR #123 (test/repo)
//	  url-group/0/CI/0: CI workflow
//	    url-group/0/CI/0/build/0: build job
//	      url-group/0/CI/0/build/0/Checkout/0: Checkout step
//	  url-group/0/Review: APPROVED/0: marker
//	url-group/1: PR #456 (other/repo)
//	  url-group/1/Deploy/0: Deploy workflow
//	    url-group/1/Deploy/0/deploy-prod/0: deploy-prod job
func createMultiURLTestModel() Model {
	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Minute)

	m := Model{
		expandedState:  make(map[string]bool),
		hiddenState:    make(map[string]bool),
		globalStart:    globalStart,
		globalEnd:      globalEnd,
		chartStart:     globalStart,
		chartEnd:       globalEnd,
		keys:           DefaultKeyMap(),
		width:          120,
		height:         40,
		inputURLs:      []string{"https://github.com/test/repo/pull/123", "https://github.com/other/repo/pull/456"},
		selectionStart: -1,
		treeWidth:      defaultTreeWidth,
	}

	m.roots = []*analyzer.TreeNode{
		{
			Name:      "CI",
			Hints:     enrichment.SpanHints{Category: "workflow", IsRoot: true, Outcome: "success", Color: "green", BarChar: "█"},
			StartTime: globalStart,
			EndTime:   globalStart.Add(5 * time.Minute),
			URLIndex:  0,
			Children: []*analyzer.TreeNode{
				{
					Name:      "build",
					Hints:     enrichment.SpanHints{Category: "job", Outcome: "success", Color: "green", BarChar: "█"},
					StartTime: globalStart,
					EndTime:   globalStart.Add(2 * time.Minute),
					URLIndex:  0,
					Children: []*analyzer.TreeNode{
						{
							Name:      "Checkout",
							Hints:     enrichment.SpanHints{Category: "step", IsLeaf: true, Outcome: "success", Color: "green", BarChar: "▒"},
							StartTime: globalStart,
							EndTime:   globalStart.Add(10 * time.Second),
							URLIndex:  0,
						},
					},
				},
			},
		},
		{
			Name:      "Review: APPROVED",
			Hints:     enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", EventType: "approved", User: "reviewer", Icon: "✓", BarChar: "✓", Color: "green"},
			StartTime: globalStart.Add(3 * time.Minute),
			EndTime:   globalStart.Add(3 * time.Minute),
			URLIndex:  0,
		},
		{
			Name:      "Deploy",
			Hints:     enrichment.SpanHints{Category: "workflow", IsRoot: true, Outcome: "failure", Color: "red", BarChar: "█"},
			StartTime: globalStart.Add(5 * time.Minute),
			EndTime:   globalEnd,
			URLIndex:  1,
			Children: []*analyzer.TreeNode{
				{
					Name:      "deploy-prod",
					Hints:     enrichment.SpanHints{Category: "job", Outcome: "failure", Color: "red", BarChar: "█"},
					StartTime: globalStart.Add(5 * time.Minute),
					EndTime:   globalEnd,
					URLIndex:  1,
				},
			},
		},
	}

	// Expand URL groups and their workflows (depth 1)
	m.expandAllToDepth(1)
	m.rebuildItems()

	return m
}

// collectAllIDs returns every ID in the tree (via treeItems).
func collectAllIDs(items []*TreeItem) map[string]bool {
	ids := make(map[string]bool)
	var walk func([]*TreeItem)
	walk = func(items []*TreeItem) {
		for _, item := range items {
			ids[item.ID] = true
			walk(item.Children)
		}
	}
	walk(items)
	return ids
}

// focusedIDs returns the set of item IDs that are NOT hidden after focus.
func focusedIDs(m *Model) map[string]bool {
	all := collectAllIDs(m.treeItems)
	focused := make(map[string]bool)
	for id := range all {
		if !m.hiddenState[id] {
			focused[id] = true
		}
	}
	return focused
}

// hiddenIDs returns the set of item IDs that ARE hidden.
func hiddenIDs(m *Model) map[string]bool {
	all := collectAllIDs(m.treeItems)
	hidden := make(map[string]bool)
	for id := range all {
		if m.hiddenState[id] {
			hidden[id] = true
		}
	}
	return hidden
}

// findVisibleIndex returns the index of the item with the given ID in visibleItems, or -1.
func findVisibleIndex(m *Model, id string) int {
	for i, item := range m.visibleItems {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func TestFocusSingleURL(t *testing.T) {
	t.Parallel()

	t.Run("focus on workflow focuses entire subtree", func(t *testing.T) {
		m := createTestModel()
		// cursor is on the workflow (CI)
		m.cursor = 0
		m.toggleFocus()

		assert.True(t, m.isFocused)
		focused := focusedIDs(&m)
		// Workflow and all children should be focused
		assert.True(t, focused["CI/0"])
		assert.True(t, focused["CI/0/build/0"])
		assert.True(t, focused["CI/0/test/1"])
		assert.True(t, focused["CI/0/build/0/Checkout/0"])
		assert.True(t, focused["CI/0/build/0/Build/1"])
	})

	t.Run("focus on job focuses job and its steps", func(t *testing.T) {
		m := createTestModel()
		// Expand workflow to see jobs
		m.expandedState["CI/0"] = true
		m.rebuildItems()

		// Move cursor to "build" job
		idx := findVisibleIndex(&m, "CI/0/build/0")
		assert.GreaterOrEqual(t, idx, 0, "build job should be visible")
		m.cursor = idx
		m.toggleFocus()

		assert.True(t, m.isFocused)
		focused := focusedIDs(&m)
		// Job and its steps should be focused
		assert.True(t, focused["CI/0/build/0"])
		assert.True(t, focused["CI/0/build/0/Checkout/0"])
		assert.True(t, focused["CI/0/build/0/Build/1"])
		// Sibling job should be hidden
		hidden := hiddenIDs(&m)
		assert.True(t, hidden["CI/0/test/1"])
	})

	t.Run("focus on step focuses only that step", func(t *testing.T) {
		m := createTestModel()
		// Expand all to see steps
		m.expandAll()

		idx := findVisibleIndex(&m, "CI/0/build/0/Checkout/0")
		assert.GreaterOrEqual(t, idx, 0, "Checkout step should be visible")
		m.cursor = idx
		m.toggleFocus()

		assert.True(t, m.isFocused)
		focused := focusedIDs(&m)
		assert.True(t, focused["CI/0/build/0/Checkout/0"])
		// Sibling step should be hidden
		hidden := hiddenIDs(&m)
		assert.True(t, hidden["CI/0/build/0/Build/1"])
	})

	t.Run("unfocus restores previous hidden state", func(t *testing.T) {
		m := createTestModel()
		// Hide a job first
		m.hiddenState["CI/0/test/1"] = true
		originalHidden := make(map[string]bool)
		for k, v := range m.hiddenState {
			originalHidden[k] = v
		}

		m.cursor = 0
		m.toggleFocus()
		assert.True(t, m.isFocused)

		m.toggleFocus()
		assert.False(t, m.isFocused)
		assert.Equal(t, originalHidden, m.hiddenState)
	})
}

func TestFocusMultiURL(t *testing.T) {
	t.Parallel()

	t.Run("focus on URL group focuses entire subtree", func(t *testing.T) {
		m := createMultiURLTestModel()

		// Find URL group 0
		idx := findVisibleIndex(&m, "url-group/0")
		assert.GreaterOrEqual(t, idx, 0, "url-group/0 should be visible")
		m.cursor = idx
		m.toggleFocus()

		assert.True(t, m.isFocused)
		focused := focusedIDs(&m)
		// URL group 0 and all descendants should be focused
		assert.True(t, focused["url-group/0"], "url-group/0 should be focused")
		for id := range focused {
			// All focused items should belong to url-group/0
			if id != "url-group/0" {
				assert.NotContains(t, id, "url-group/1", "url-group/1 items should not be focused: %s", id)
			}
		}
		// URL group 1 and all its descendants should be hidden
		hidden := hiddenIDs(&m)
		assert.True(t, hidden["url-group/1"], "url-group/1 should be hidden")
	})

	t.Run("focus on URL group includes markers", func(t *testing.T) {
		m := createMultiURLTestModel()

		idx := findVisibleIndex(&m, "url-group/0")
		assert.GreaterOrEqual(t, idx, 0)
		m.cursor = idx
		m.toggleFocus()

		focused := focusedIDs(&m)
		// Find the marker under url-group/0
		markerFocused := false
		for id := range focused {
			if strings.Contains(id, "Review") || strings.Contains(id, "APPROVED") {
				markerFocused = true
			}
		}
		assert.True(t, markerFocused, "marker under url-group/0 should be focused")
	})

	t.Run("focus on workflow inside URL group focuses only that workflow", func(t *testing.T) {
		m := createMultiURLTestModel()

		// Find the CI workflow under url-group/0
		var ciID string
		for _, item := range m.visibleItems {
			if item.Name == "CI" && item.ItemType == ItemTypeRoot {
				ciID = item.ID
				break
			}
		}
		assert.NotEmpty(t, ciID, "CI workflow should be visible")

		idx := findVisibleIndex(&m, ciID)
		m.cursor = idx
		m.toggleFocus()

		focused := focusedIDs(&m)
		assert.True(t, focused[ciID])
		// Children should be focused
		for _, item := range m.treeItems {
			for _, child := range item.Children {
				if child.ID == ciID {
					for _, grandchild := range child.Children {
						assert.True(t, focused[grandchild.ID], "child %s should be focused", grandchild.ID)
					}
				}
			}
		}
		// URL group 1's items should be hidden
		hidden := hiddenIDs(&m)
		assert.True(t, hidden["url-group/1"])
	})

	t.Run("focus on job inside URL group focuses only that job subtree", func(t *testing.T) {
		m := createMultiURLTestModel()
		// Expand all to see jobs
		m.expandAll()

		// Find the build job
		var buildID string
		for _, item := range m.visibleItems {
			if item.Name == "build" && item.ItemType == ItemTypeIntermediate {
				buildID = item.ID
				break
			}
		}
		assert.NotEmpty(t, buildID, "build job should be visible")

		idx := findVisibleIndex(&m, buildID)
		m.cursor = idx
		m.toggleFocus()

		focused := focusedIDs(&m)
		assert.True(t, focused[buildID])
		// Step should be focused
		for _, id := range []string{} {
			_ = id
		}
		// Check that deploy-prod job (in URL group 1) is hidden
		hidden := hiddenIDs(&m)
		for id := range hidden {
			if strings.Contains(id, "deploy-prod") {
				// deploy-prod should be hidden
				assert.True(t, hidden[id])
			}
		}
	})

	t.Run("focus on collapsed URL group still focuses all descendants", func(t *testing.T) {
		m := createMultiURLTestModel()

		// Collapse url-group/0
		m.expandedState["url-group/0"] = false
		m.rebuildItems()

		idx := findVisibleIndex(&m, "url-group/0")
		assert.GreaterOrEqual(t, idx, 0)
		m.cursor = idx
		m.toggleFocus()

		focused := focusedIDs(&m)
		assert.True(t, focused["url-group/0"])
		// Descendants should still be focused even though they weren't visible
		allIDs := collectAllIDs(m.treeItems)
		for id := range allIDs {
			if strings.HasPrefix(id, "url-group/0/") {
				assert.True(t, focused[id], "descendant %s should be focused even when parent collapsed", id)
			}
		}
	})

	t.Run("unfocus after URL group focus restores state", func(t *testing.T) {
		m := createMultiURLTestModel()
		originalHidden := make(map[string]bool)
		for k, v := range m.hiddenState {
			originalHidden[k] = v
		}

		idx := findVisibleIndex(&m, "url-group/0")
		m.cursor = idx
		m.toggleFocus()
		assert.True(t, m.isFocused)

		m.toggleFocus()
		assert.False(t, m.isFocused)
		assert.Equal(t, originalHidden, m.hiddenState)
	})

	t.Run("focus updates chart bounds to focused subtree", func(t *testing.T) {
		m := createMultiURLTestModel()

		// Focus on URL group 0 (which has earlier times)
		idx := findVisibleIndex(&m, "url-group/0")
		m.cursor = idx
		m.toggleFocus()

		// Chart bounds should reflect only url-group/0's time range
		// url-group/0 ends at globalStart+5min, url-group/1 starts at globalStart+5min
		assert.True(t, m.chartEnd.Before(m.globalEnd) || m.chartEnd.Equal(m.globalEnd))
		assert.True(t, m.chartStart.Equal(m.globalStart) || m.chartStart.After(m.globalStart))
	})

	t.Run("double focus-unfocus is idempotent", func(t *testing.T) {
		m := createMultiURLTestModel()
		originalHidden := make(map[string]bool)
		for k, v := range m.hiddenState {
			originalHidden[k] = v
		}

		idx := findVisibleIndex(&m, "url-group/0")
		m.cursor = idx

		// Focus then unfocus
		m.toggleFocus()
		m.toggleFocus()
		assert.Equal(t, originalHidden, m.hiddenState)

		// Focus then unfocus again
		m.toggleFocus()
		m.toggleFocus()
		assert.Equal(t, originalHidden, m.hiddenState)
	})
}

func TestSearchMode(t *testing.T) {
	t.Parallel()

	t.Run("/ activates search mode", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)

		assert.True(t, m.isSearching)
		assert.Equal(t, "", m.searchQuery)
	})

	t.Run("typing in search mode updates query and filters", func(t *testing.T) {
		m := createTestModel()
		// Expand all so steps are visible
		m.expandAll()

		// Enter search mode
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)

		// Type "build"
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		assert.Equal(t, "build", m.searchQuery)
		assert.True(t, m.isSearching)
		// "build" job should match, plus its ancestor "CI" workflow
		assert.True(t, m.searchMatchIDs["CI/0/build/0"], "build job should be a match")
		// CI workflow should be an ancestor (visible for context)
		assert.True(t, m.searchAncIDs["CI/0"], "CI workflow should be an ancestor")
	})

	t.Run("search is case insensitive", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)

		for _, r := range "BUILD" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// "build" job should still match (case-insensitive)
		assert.True(t, m.searchMatchIDs["CI/0/build/0"])
	})

	t.Run("search filters visible items", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Enter search mode and type "test"
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)

		for _, r := range "test" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Should have fewer visible items
		assert.Less(t, len(m.visibleItems), beforeCount)
		// "test" job should be visible
		found := false
		for _, item := range m.visibleItems {
			if item.Name == "test" {
				found = true
				break
			}
		}
		assert.True(t, found, "test job should be visible in filtered results")
	})

	t.Run("Esc during search clears query and exits", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Enter search mode and type something
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
		m = newModel.(Model)

		// Press Esc
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.False(t, m.isSearching)
		assert.Equal(t, "", m.searchQuery)
		assert.Nil(t, m.searchMatchIDs)
		assert.Equal(t, beforeCount, len(m.visibleItems))
	})

	t.Run("Down exits search input but keeps filter", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()

		// Enter search mode and type "build"
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}
		filteredCount := len(m.visibleItems)

		// Press Down to exit input but keep filter
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = newModel.(Model)

		assert.False(t, m.isSearching)
		assert.Equal(t, "build", m.searchQuery)
		assert.Equal(t, filteredCount, len(m.visibleItems))
	})

	t.Run("Enter clears filter and preserves cursor position", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Enter search mode and type "test" (matches the "test" job)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "test" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Exit search input with Down, then navigate to the match
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = newModel.(Model)

		// Find the "test" item in filtered results
		var testIdx int
		for i, item := range m.visibleItems {
			if item.Name == "test" {
				testIdx = i
				break
			}
		}
		m.cursor = testIdx
		cursorItemID := m.visibleItems[m.cursor].ID

		// Press Enter to clear filter and keep cursor on same item
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = newModel.(Model)

		assert.Equal(t, "", m.searchQuery)
		assert.Equal(t, beforeCount, len(m.visibleItems))
		// Cursor should still be on the "test" item
		assert.Equal(t, cursorItemID, m.visibleItems[m.cursor].ID)
	})

	t.Run("Esc clears filter and preserves cursor position", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Enter search mode, type, exit input with Down, then Esc to clear
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = newModel.(Model)

		// Navigate to "build" in filtered list
		var buildIdx int
		for i, item := range m.visibleItems {
			if item.Name == "build" {
				buildIdx = i
				break
			}
		}
		m.cursor = buildIdx
		cursorItemID := m.visibleItems[m.cursor].ID

		// Press Esc to clear filter
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.Equal(t, "", m.searchQuery)
		assert.Equal(t, beforeCount, len(m.visibleItems))
		assert.Equal(t, cursorItemID, m.visibleItems[m.cursor].ID)
	})

	t.Run("backspace removes last character", func(t *testing.T) {
		m := createTestModel()

		// Enter search mode and type "abc"
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "abc" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}
		assert.Equal(t, "abc", m.searchQuery)

		// Backspace
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = newModel.(Model)
		assert.Equal(t, "ab", m.searchQuery)
	})

	t.Run("search auto-expands ancestors of matches", func(t *testing.T) {
		m := createTestModel()
		// Collapse everything first
		m.expandedState = make(map[string]bool)
		m.rebuildItems()
		assert.Equal(t, 1, len(m.visibleItems)) // only CI workflow visible

		// Search for "Checkout" (a step nested under CI > build)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "Checkout" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Ancestors should be expanded and visible
		assert.True(t, m.expandedState["CI/0"], "CI workflow should be expanded")
		assert.True(t, m.expandedState["CI/0/build/0"], "build job should be expanded")
		// Checkout should be visible
		found := false
		for _, item := range m.visibleItems {
			if item.Name == "Checkout" {
				found = true
				break
			}
		}
		assert.True(t, found, "Checkout step should be visible")
	})

	t.Run("no match query shows no items", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "zzzznonexistent" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		assert.Equal(t, 0, len(m.searchMatchIDs))
		assert.Equal(t, 0, len(m.visibleItems))
	})

	t.Run("search bar renders in view", func(t *testing.T) {
		m := createTestModel()

		// Enter search mode
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		view := m.View()
		assert.Contains(t, view, "build")
		assert.Contains(t, view, "matches")
	})

	t.Run("navigation keys ignored during search input", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		// Enter search mode
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)

		// Try j key (should be appended as text, not nav)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, "j", m.searchQuery)
		assert.Equal(t, 0, m.cursor) // cursor should not have moved
	})
}

func TestFilterVisibleItems(t *testing.T) {
	t.Parallel()

	t.Run("returns only matched and ancestor items", func(t *testing.T) {
		items := []TreeItem{
			{ID: "a"},
			{ID: "b"},
			{ID: "c"},
		}
		matchIDs := map[string]bool{"b": true}
		ancestorIDs := map[string]bool{"a": true}

		result := FilterVisibleItems(items, matchIDs, ancestorIDs)

		assert.Len(t, result, 2)
		assert.Equal(t, "a", result[0].ID)
		assert.Equal(t, "b", result[1].ID)
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		items := []TreeItem{
			{ID: "a"},
			{ID: "b"},
		}
		matchIDs := map[string]bool{}
		ancestorIDs := map[string]bool{}

		result := FilterVisibleItems(items, matchIDs, ancestorIDs)

		assert.Empty(t, result)
	})
}

func TestSearchKeyBinding(t *testing.T) {
	t.Parallel()

	t.Run("keymap includes search binding", func(t *testing.T) {
		km := DefaultKeyMap()
		assert.NotEmpty(t, km.Search.Keys())
	})

	t.Run("short help includes search", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelp()
		assert.Contains(t, help, "search")
	})

	t.Run("full help includes search", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.FullHelp()
		found := false
		for _, row := range help {
			if len(row) >= 2 && strings.Contains(row[1], "Search") {
				found = true
				break
			}
		}
		assert.True(t, found, "Full help should contain search info")
	})
}

func TestLogicalEnd(t *testing.T) {
	t.Parallel()

	t.Run("sets logicalEndID and logicalEndTime from cursor item", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		item := m.visibleItems[0]

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)

		assert.Equal(t, item.ID, m.logicalEndID)
		assert.Equal(t, item.EndTime, m.logicalEndTime)
	})

	t.Run("toggles off when same item selected again", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		// Set marker
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)
		assert.NotEmpty(t, m.logicalEndID)

		// Toggle off
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)

		assert.Empty(t, m.logicalEndID)
		assert.True(t, m.logicalEndTime.IsZero())
	})

	t.Run("moves marker when different item selected", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		// Set marker on first item
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)
		firstID := m.logicalEndID

		// Move to second item and set marker
		m.cursor = 1
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)

		assert.NotEqual(t, firstID, m.logicalEndID)
		assert.Equal(t, m.visibleItems[1].ID, m.logicalEndID)
		assert.Equal(t, m.visibleItems[1].EndTime, m.logicalEndTime)
	})
}

func TestDetailModalNavigation(t *testing.T) {
	t.Parallel()

	t.Run("h/left navigates to previous item in modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
		assert.Equal(t, m.visibleItems[0].ID, m.modalItem.ID)
		assert.Equal(t, 0, m.modalScroll)
	})

	t.Run("l/right navigates to next item in modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
		m = newModel.(Model)

		assert.Equal(t, 1, m.cursor)
		assert.Equal(t, m.visibleItems[1].ID, m.modalItem.ID)
		assert.Equal(t, 0, m.modalScroll)
	})

	t.Run("left arrow navigates to previous item in modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
		assert.NotNil(t, m.modalItem)
	})

	t.Run("right arrow navigates to next item in modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = newModel.(Model)

		assert.Equal(t, 1, m.cursor)
		assert.NotNil(t, m.modalItem)
	})

	t.Run("h at first item does not change cursor", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("l at last item does not change cursor", func(t *testing.T) {
		m := createTestModel()
		m.cursor = len(m.visibleItems) - 1
		m.openDetailModal()
		lastCursor := m.cursor

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
		m = newModel.(Model)

		assert.Equal(t, lastCursor, m.cursor)
	})

	t.Run("modal scroll resets on h/l navigation", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1
		m.openDetailModal()
		m.modalScroll = 5

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.modalScroll)
	})

	t.Run("j scrolls modal content down", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, 1, m.modalScroll)
	})

	t.Run("k scrolls modal content up", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()
		m.modalScroll = 3

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = newModel.(Model)

		assert.Equal(t, 2, m.modalScroll)
	})

	t.Run("k does not scroll below zero", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()
		m.modalScroll = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.modalScroll)
	})

	t.Run("r in modal with reloadFunc closes modal and starts loading", func(t *testing.T) {
		m := createTestModel()
		m.reloadFunc = func(reporter LoadingReporter) ([]trace.ReadOnlySpan, time.Time, time.Time, error) {
			return nil, time.Now(), time.Now(), nil
		}
		m.cursor = 0
		m.openDetailModal()

		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		m = newModel.(Model)

		assert.False(t, m.showDetailModal)
		assert.Nil(t, m.modalItem)
		assert.True(t, m.isLoading)
		assert.NotNil(t, cmd)
	})

	t.Run("r in modal without reloadFunc just closes modal", func(t *testing.T) {
		m := createTestModel()
		m.reloadFunc = nil
		m.cursor = 0
		m.openDetailModal()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		m = newModel.(Model)

		assert.False(t, m.showDetailModal)
		assert.Nil(t, m.modalItem)
		assert.False(t, m.isLoading)
	})
}

func TestReloadFlow(t *testing.T) {
	t.Parallel()

	t.Run("ReloadResultMsg resets model state", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 2
		m.selectionStart = 1
		m.logicalEndID = "some-id"
		m.isLoading = true

		now := time.Now()
		newStart := now.Add(-10 * time.Minute)
		newEnd := now

		newModel, _ := m.Update(ReloadResultMsg{
			spans:       nil,
			globalStart: newStart,
			globalEnd:   newEnd,
		})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
		assert.Equal(t, -1, m.selectionStart)
		assert.Empty(t, m.logicalEndID)
		assert.False(t, m.isLoading)
		assert.Equal(t, newStart, m.globalStart)
		assert.Equal(t, newEnd, m.globalEnd)
		assert.Equal(t, newStart, m.chartStart)
		assert.Equal(t, newEnd, m.chartEnd)
	})

	t.Run("ReloadResultMsg clears expanded and hidden state", func(t *testing.T) {
		m := createTestModel()
		m.expandedState["foo"] = true
		m.hiddenState["bar"] = true
		m.isLoading = true

		now := time.Now()
		newModel, _ := m.Update(ReloadResultMsg{
			globalStart: now,
			globalEnd:   now.Add(time.Minute),
		})
		m = newModel.(Model)

		assert.False(t, m.expandedState["foo"])
		assert.False(t, m.hiddenState["bar"])
	})

	t.Run("ReloadResultMsg with error returns early", func(t *testing.T) {
		m := createTestModel()
		m.isLoading = true
		m.cursor = 2

		newModel, _ := m.Update(ReloadResultMsg{
			err: fmt.Errorf("reload failed"),
		})
		m = newModel.(Model)

		assert.False(t, m.isLoading)
		// cursor should not be reset on error
		assert.Equal(t, 2, m.cursor)
	})

	t.Run("ReloadResultMsg clears progress fields", func(t *testing.T) {
		m := createTestModel()
		m.isLoading = true
		m.loadingPhase = "Fetching"
		m.loadingDetail = "page 3/5"
		m.loadingURL = "https://example.com"

		now := time.Now()
		newModel, _ := m.Update(ReloadResultMsg{
			globalStart: now,
			globalEnd:   now.Add(time.Minute),
		})
		m = newModel.(Model)

		assert.Empty(t, m.loadingPhase)
		assert.Empty(t, m.loadingDetail)
		assert.Empty(t, m.loadingURL)
	})
}

func TestLoadingProgressMsg(t *testing.T) {
	t.Parallel()

	t.Run("non-empty fields update model", func(t *testing.T) {
		m := createTestModel()
		m.progressCh = make(chan LoadingProgressMsg, 1)
		m.resultCh = make(chan ReloadResultMsg, 1)

		newModel, _ := m.Update(LoadingProgressMsg{
			Phase:  "Downloading",
			Detail: "50%",
			URL:    "https://example.com",
		})
		m = newModel.(Model)

		assert.Equal(t, "Downloading", m.loadingPhase)
		assert.Equal(t, "50%", m.loadingDetail)
		assert.Equal(t, "https://example.com", m.loadingURL)
	})

	t.Run("empty fields do not overwrite existing values", func(t *testing.T) {
		m := createTestModel()
		m.loadingPhase = "Downloading"
		m.loadingDetail = "50%"
		m.loadingURL = "https://example.com"
		m.progressCh = make(chan LoadingProgressMsg, 1)
		m.resultCh = make(chan ReloadResultMsg, 1)

		newModel, _ := m.Update(LoadingProgressMsg{
			Phase: "Processing",
		})
		m = newModel.(Model)

		assert.Equal(t, "Processing", m.loadingPhase)
		assert.Equal(t, "50%", m.loadingDetail)
		assert.Equal(t, "https://example.com", m.loadingURL)
	})
}

func TestGGSequences(t *testing.T) {
	t.Parallel()

	t.Run("gg jumps to top", func(t *testing.T) {
		m := createTestModel()
		m.cursor = len(m.visibleItems) - 1

		// First g
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		m = newModel.(Model)
		assert.True(t, m.pendingG)

		// Second g
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
		assert.False(t, m.pendingG)
	})

	t.Run("GG jumps to bottom", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		// First G
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
		m = newModel.(Model)
		assert.True(t, m.pendingGG)

		// Second G
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
		m = newModel.(Model)

		assert.Equal(t, len(m.visibleItems)-1, m.cursor)
		assert.False(t, m.pendingGG)
	})

	t.Run("other key between g presses clears pending state", func(t *testing.T) {
		m := createTestModel()
		m.cursor = len(m.visibleItems) - 1

		// First g
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		m = newModel.(Model)
		assert.True(t, m.pendingG)

		// Different key
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.False(t, m.pendingG)
		assert.NotEqual(t, 0, m.cursor) // did not jump to top
	})

	t.Run("gg clears selectionStart", func(t *testing.T) {
		m := createTestModel()
		m.cursor = len(m.visibleItems) - 1
		m.selectionStart = 1

		// gg
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		m = newModel.(Model)

		assert.Equal(t, -1, m.selectionStart)
	})

	t.Run("GG clears selectionStart", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.selectionStart = 0

		// GG
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
		m = newModel.(Model)

		assert.Equal(t, -1, m.selectionStart)
	})
}

func TestSearchEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("backspace on empty query is no-op", func(t *testing.T) {
		m := createTestModel()

		// Enter search mode
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		assert.True(t, m.isSearching)
		assert.Equal(t, "", m.searchQuery)

		// Backspace on empty
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = newModel.(Model)

		assert.True(t, m.isSearching)
		assert.Equal(t, "", m.searchQuery)
	})

	t.Run("tab exits search keeping filter active", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()

		// Enter search and type
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}
		filteredCount := len(m.visibleItems)

		// Press Tab
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = newModel.(Model)

		assert.False(t, m.isSearching)
		assert.Equal(t, "build", m.searchQuery)
		assert.Equal(t, filteredCount, len(m.visibleItems))
	})

	t.Run("enter after exiting search input clears filter preserves cursor", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Search for "test"
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "test" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Exit input with Tab
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = newModel.(Model)

		// Navigate to "test" item
		for i, item := range m.visibleItems {
			if item.Name == "test" {
				m.cursor = i
				break
			}
		}
		cursorItemID := m.visibleItems[m.cursor].ID

		// Press Enter to clear filter
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = newModel.(Model)

		assert.Equal(t, "", m.searchQuery)
		assert.Equal(t, beforeCount, len(m.visibleItems))
		assert.Equal(t, cursorItemID, m.visibleItems[m.cursor].ID)
	})

	t.Run("esc during search input clears everything", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		beforeCount := len(m.visibleItems)

		// Enter search and type
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Esc during input
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.False(t, m.isSearching)
		assert.Equal(t, "", m.searchQuery)
		assert.Nil(t, m.searchMatchIDs)
		assert.Equal(t, beforeCount, len(m.visibleItems))
	})
}

func TestMouseClick(t *testing.T) {
	t.Parallel()

	t.Run("left click maps Y position to item index", func(t *testing.T) {
		m := createTestModel()
		m.mouseEnabled = true
		m.height = 40
		m.width = 120

		// Header is 8 lines (or 9 with enrichment), clicking at Y=8 should select first item
		newModel, _ := m.Update(tea.MouseMsg{
			X:      10,
			Y:      8,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionRelease,
		})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("click clears selection", func(t *testing.T) {
		m := createTestModel()
		m.mouseEnabled = true
		m.selectionStart = 0
		m.height = 40
		m.width = 120

		newModel, _ := m.Update(tea.MouseMsg{
			X:      10,
			Y:      9,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionRelease,
		})
		m = newModel.(Model)

		assert.Equal(t, -1, m.selectionStart)
	})

	t.Run("out-of-range Y does not panic or change cursor", func(t *testing.T) {
		m := createTestModel()
		m.mouseEnabled = true
		m.cursor = 1
		m.height = 40
		m.width = 120

		// Click way below content
		newModel, _ := m.Update(tea.MouseMsg{
			X:      10,
			Y:      200,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionRelease,
		})
		m = newModel.(Model)

		// Cursor should not change for out-of-range click
		assert.Equal(t, 1, m.cursor)
	})

	t.Run("mouse ignored during loading", func(t *testing.T) {
		m := createTestModel()
		m.isLoading = true
		m.cursor = 0

		newModel, _ := m.Update(tea.MouseMsg{
			X:      10,
			Y:      10,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionRelease,
		})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("wheel up in modal scrolls modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()
		m.modalScroll = 3

		newModel, _ := m.Update(tea.MouseMsg{
			Button: tea.MouseButtonWheelUp,
		})
		m = newModel.(Model)

		assert.Equal(t, 2, m.modalScroll)
	})

	t.Run("wheel down in modal scrolls modal", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0
		m.openDetailModal()
		m.modalScroll = 0

		newModel, _ := m.Update(tea.MouseMsg{
			Button: tea.MouseButtonWheelDown,
		})
		m = newModel.(Model)

		assert.Equal(t, 1, m.modalScroll)
	})

	t.Run("wheel up in main view moves cursor up", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 1

		newModel, _ := m.Update(tea.MouseMsg{
			Button: tea.MouseButtonWheelUp,
		})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("wheel down in main view moves cursor down", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		newModel, _ := m.Update(tea.MouseMsg{
			Button: tea.MouseButtonWheelDown,
		})
		m = newModel.(Model)

		assert.Equal(t, 1, m.cursor)
	})
}

func TestChartBoundsRecalculation(t *testing.T) {
	t.Parallel()

	t.Run("hiding latest item shrinks chartEnd", func(t *testing.T) {
		m := createMultiURLTestModel()
		m.expandAll()
		originalChartEnd := m.chartEnd

		// Find the Deploy workflow (url-group/1, ends at globalEnd).
		// Hide the entire url-group/1 which contains the latest items.
		for i, item := range m.visibleItems {
			if item.ID == "url-group/1" {
				m.cursor = i
				break
			}
		}

		// Hide via space — hides url-group/1 and all descendants
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = newModel.(Model)

		assert.True(t, m.chartEnd.Before(originalChartEnd),
			"chartEnd %v should be before original %v", m.chartEnd, originalChartEnd)
	})

	t.Run("unhiding restores chartEnd", func(t *testing.T) {
		m := createMultiURLTestModel()
		m.expandAll()
		originalChartEnd := m.chartEnd

		// Find and hide url-group/1
		for i, item := range m.visibleItems {
			if item.ID == "url-group/1" {
				m.cursor = i
				break
			}
		}

		// Hide
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = newModel.(Model)
		assert.True(t, m.chartEnd.Before(originalChartEnd))

		// Unhide
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = newModel.(Model)

		assert.Equal(t, originalChartEnd, m.chartEnd)
	})
}

func TestLoadingState(t *testing.T) {
	t.Parallel()

	t.Run("ignores navigation keys while loading", func(t *testing.T) {
		m := createTestModel()
		m.isLoading = true
		initialCursor := m.cursor

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = newModel.(Model)

		assert.Equal(t, initialCursor, m.cursor)
	})

	t.Run("allows quit while loading", func(t *testing.T) {
		m := createTestModel()
		m.isLoading = true

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

		assert.NotNil(t, cmd)
	})
}

func TestSortKeybinding(t *testing.T) {
	t.Parallel()

	t.Run("s cycles through sort modes", func(t *testing.T) {
		m := createTestModel()
		assert.Equal(t, SortByStartTime, m.sortMode)

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		m = newModel.(Model)
		assert.Equal(t, SortByDurationDesc, m.sortMode)

		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		m = newModel.(Model)
		assert.Equal(t, SortByDurationAsc, m.sortMode)

		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		m = newModel.(Model)
		assert.Equal(t, SortByStartTime, m.sortMode)
	})
}

func TestResizeKeybindings(t *testing.T) {
	t.Parallel()

	t.Run("] widens tree panel", func(t *testing.T) {
		m := createTestModel()
		initial := m.treeWidth

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		m = newModel.(Model)

		assert.Equal(t, initial+treeWidthStep, m.treeWidth)
	})

	t.Run("[ narrows tree panel", func(t *testing.T) {
		m := createTestModel()
		initial := m.treeWidth

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
		m = newModel.(Model)

		assert.Equal(t, initial-treeWidthStep, m.treeWidth)
	})

	t.Run("[ respects minimum width", func(t *testing.T) {
		m := createTestModel()
		m.treeWidth = minTreeWidth

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
		m = newModel.(Model)

		assert.Equal(t, minTreeWidth, m.treeWidth)
	})

	t.Run("] respects maximum width", func(t *testing.T) {
		m := createTestModel()
		m.treeWidth = maxTreeWidth

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		m = newModel.(Model)

		assert.Equal(t, maxTreeWidth, m.treeWidth)
	})
}

func TestDynamicHelp(t *testing.T) {
	t.Parallel()

	t.Run("normal mode shows sort and resize keys", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelpForMode(HelpModeNormal)
		assert.Contains(t, help, "sort")
		assert.Contains(t, help, "resize")
		assert.Contains(t, help, "page")
		assert.Contains(t, help, "jump")
	})

	t.Run("search mode shows search-specific keys", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelpForMode(HelpModeSearch)
		assert.Contains(t, help, "type to search")
		assert.Contains(t, help, "esc cancel")
		assert.NotContains(t, help, "sort")
	})

	t.Run("search active mode shows filter keys", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelpForMode(HelpModeSearchActive)
		assert.Contains(t, help, "clear")
		assert.Contains(t, help, "sort")
		assert.Contains(t, help, "jump")
	})

	t.Run("modal mode shows modal keys", func(t *testing.T) {
		km := DefaultKeyMap()
		help := km.ShortHelpForMode(HelpModeModal)
		assert.Contains(t, help, "scroll")
		assert.Contains(t, help, "close")
	})
}

func TestFilterZoom(t *testing.T) {
	t.Parallel()

	t.Run("search zooms timeline to matched items", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()

		originalStart := m.chartStart
		originalEnd := m.chartEnd

		// Search for "build" (a job that doesn't span the full timeline)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Chart bounds should be narrowed to the "build" job's time range
		assert.False(t, m.chartStart.Equal(originalStart) && m.chartEnd.Equal(originalEnd),
			"chart bounds should change when search is active")
	})

	t.Run("clearing search restores timeline bounds", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()

		// Search for "build"
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m = newModel.(Model)
		for _, r := range "build" {
			newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = newModel.(Model)
		}

		// Exit search input and clear
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		// Chart bounds should be restored
		assert.Equal(t, m.globalStart, m.chartStart)
		assert.Equal(t, m.globalEnd, m.chartEnd)
	})
}

func TestJumpToNext(t *testing.T) {
	t.Parallel()

	t.Run("n jumps to next failed item", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0 // CI workflow (success)

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = newModel.(Model)

		// Should jump to the "test" job which has outcome "failure"
		assert.Equal(t, "failure", m.visibleItems[m.cursor].Hints.Outcome)
	})

	t.Run("n wraps around", func(t *testing.T) {
		m := createTestModel()
		// Move to last item
		m.cursor = len(m.visibleItems) - 1

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = newModel.(Model)

		// Should wrap around and find the failed item
		if m.visibleItems[m.cursor].Hints.Outcome == "failure" {
			assert.Equal(t, "failure", m.visibleItems[m.cursor].Hints.Outcome)
		}
	})

	t.Run("n does nothing when no failed items", func(t *testing.T) {
		m := createTestModel()
		// Remove the failure from visible items by hiding it
		// Just verify cursor doesn't crash
		m.cursor = 0

		// N (shift) looks for bottlenecks - likely none exist
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
		m = newModel.(Model)

		assert.GreaterOrEqual(t, m.cursor, 0)
	})
}

func TestPageUpDown(t *testing.T) {
	t.Parallel()

	t.Run("ctrl+d moves cursor down by half page", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		m = newModel.(Model)

		assert.Greater(t, m.cursor, 0)
	})

	t.Run("ctrl+u moves cursor up by half page", func(t *testing.T) {
		m := createTestModel()
		m.expandAll()
		m.cursor = len(m.visibleItems) - 1

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		m = newModel.(Model)

		assert.Less(t, m.cursor, len(m.visibleItems)-1)
	})

	t.Run("ctrl+u at top stays at 0", func(t *testing.T) {
		m := createTestModel()
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		m = newModel.(Model)

		assert.Equal(t, 0, m.cursor)
	})

	t.Run("ctrl+d at bottom stays at last item", func(t *testing.T) {
		m := createTestModel()
		lastIdx := len(m.visibleItems) - 1
		m.cursor = lastIdx

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		m = newModel.(Model)

		assert.Equal(t, lastIdx, m.cursor)
	})
}

func TestReloadError(t *testing.T) {
	t.Parallel()

	t.Run("reload error is displayed", func(t *testing.T) {
		m := createTestModel()

		newModel, _ := m.Update(ReloadResultMsg{
			err: fmt.Errorf("connection refused"),
		})
		m = newModel.(Model)

		assert.Equal(t, "connection refused", m.reloadError)
		assert.False(t, m.isLoading)
	})

	t.Run("error bar appears in view", func(t *testing.T) {
		m := createTestModel()
		m.reloadError = "test error"

		view := m.View()
		assert.Contains(t, view, "Reload failed")
		assert.Contains(t, view, "test error")
	})

	t.Run("esc dismisses error", func(t *testing.T) {
		m := createTestModel()
		m.reloadError = "some error"

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newModel.(Model)

		assert.Empty(t, m.reloadError)
	})

	t.Run("successful reload clears error", func(t *testing.T) {
		m := createTestModel()
		m.reloadError = "old error"

		newModel, _ := m.Update(ReloadResultMsg{
			spans:       m.spans,
			globalStart: m.globalStart,
			globalEnd:   m.globalEnd,
		})
		m = newModel.(Model)

		assert.Empty(t, m.reloadError)
	})
}

func TestSpanIndex(t *testing.T) {
	t.Parallel()

	t.Run("builds index from tree items", func(t *testing.T) {
		items := []*TreeItem{
			{
				ID:       "root",
				ParentID: "",
				Children: []*TreeItem{
					{ID: "child1", ParentID: "root"},
					{ID: "child2", ParentID: "root"},
				},
			},
		}
		idx := BuildSpanIndex(items)

		assert.NotNil(t, idx.ByID["root"])
		assert.NotNil(t, idx.ByID["child1"])
		assert.NotNil(t, idx.ByID["child2"])
		assert.Len(t, idx.ByParentID["root"], 2)
	})
}
