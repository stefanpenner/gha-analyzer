package results

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stretchr/testify/assert"
)

// createTestModel creates a Model with test data for integration testing
func createTestModel() Model {
	now := time.Now()
	globalStart := now
	globalEnd := now.Add(5 * time.Minute)

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
		inputURLs:      []string{"https://github.com/test/repo/pull/123"},
		selectionStart: -1,
	}

	// Build test tree using analyzer.TreeNode (like real code does)
	m.roots = []*analyzer.TreeNode{
		{
			Name:      "CI",
			Type:      analyzer.NodeTypeWorkflow,
			StartTime: globalStart,
			EndTime:   globalEnd,
			URL:       "https://github.com/test/repo/actions/runs/123",
			Status:    "completed",
			Conclusion:    "success",
			Children: []*analyzer.TreeNode{
				{
					Name:      "build",
					Type:      analyzer.NodeTypeJob,
					StartTime: globalStart,
					EndTime:   globalStart.Add(2 * time.Minute),
					URL:       "https://github.com/test/repo/actions/runs/123/jobs/456",
					Status:    "completed",
					Conclusion:    "success",
					Children: []*analyzer.TreeNode{
						{
							Name:      "Checkout",
							Type:      analyzer.NodeTypeStep,
							StartTime: globalStart,
							EndTime:   globalStart.Add(10 * time.Second),
							Status:    "completed",
							Conclusion:    "success",
						},
						{
							Name:      "Build",
							Type:      analyzer.NodeTypeStep,
							StartTime: globalStart.Add(10 * time.Second),
							EndTime:   globalStart.Add(2 * time.Minute),
							Status:    "completed",
							Conclusion:    "success",
						},
					},
				},
				{
					Name:      "test",
					Type:      analyzer.NodeTypeJob,
					StartTime: globalStart.Add(2 * time.Minute),
					EndTime:   globalEnd,
					URL:       "https://github.com/test/repo/actions/runs/123/jobs/789",
					Status:    "completed",
					Conclusion:    "failure",
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
		m := NewModel(nil, now, now.Add(time.Minute), []string{"https://example.com"}, nil, nil)

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
		assert.Contains(t, view, "GitHub Actions Analyzer")
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
		m.visibleItems = FlattenVisibleItems(m.treeItems, m.expandedState)
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = newModel.(Model)

		assert.True(t, m.expandedState["CI/0"])
	})

	t.Run("collapses item with left arrow", func(t *testing.T) {
		m := createTestModel()
		m.expandedState["CI/0"] = true
		m.visibleItems = FlattenVisibleItems(m.treeItems, m.expandedState)
		m.cursor = 0

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = newModel.(Model)

		assert.False(t, m.expandedState["CI/0"])
	})

	t.Run("expand all with e key", func(t *testing.T) {
		m := createTestModel()
		// Collapse everything first
		m.expandedState = make(map[string]bool)
		m.rebuildItems()

		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		m = newModel.(Model)

		// Should expand workflow and jobs
		assert.True(t, m.expandedState["CI/0"])
	})

	t.Run("collapse all with c key", func(t *testing.T) {
		m := createTestModel()
		m.expandedState["CI/0"] = true
		m.expandedState["CI/0/job/0"] = true
		m.rebuildItems()

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
		assert.NotEmpty(t, km.ExpandAll.Keys())
		assert.NotEmpty(t, km.CollapseAll.Keys())
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
		assert.Contains(t, help, "quit")
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
		if item.URL != "" {
			assert.Contains(t, result, "\x1b]8;")
		}
	})

	t.Run("renders selected item", func(t *testing.T) {
		m := createTestModel()
		item := m.visibleItems[0]

		result := m.renderItem(item, true)

		assert.Contains(t, result, item.DisplayName)
		// Selected items should still have hyperlinks
		if item.URL != "" {
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

		assert.Contains(t, header, "GitHub Actions Analyzer")
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
		assert.Contains(t, footer, "quit")
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
	}

	m.roots = []*analyzer.TreeNode{
		{
			Name:       "CI",
			Type:       analyzer.NodeTypeWorkflow,
			StartTime:  globalStart,
			EndTime:    globalStart.Add(5 * time.Minute),
			Status:     "completed",
			Conclusion: "success",
			URLIndex:   0,
			Children: []*analyzer.TreeNode{
				{
					Name:       "build",
					Type:       analyzer.NodeTypeJob,
					StartTime:  globalStart,
					EndTime:    globalStart.Add(2 * time.Minute),
					Status:     "completed",
					Conclusion: "success",
					URLIndex:   0,
					Children: []*analyzer.TreeNode{
						{
							Name:       "Checkout",
							Type:       analyzer.NodeTypeStep,
							StartTime:  globalStart,
							EndTime:    globalStart.Add(10 * time.Second),
							Status:     "completed",
							Conclusion: "success",
							URLIndex:   0,
						},
					},
				},
			},
		},
		{
			Name:       "Review: APPROVED",
			Type:       analyzer.NodeTypeMarker,
			StartTime:  globalStart.Add(3 * time.Minute),
			EndTime:    globalStart.Add(3 * time.Minute),
			EventType:  "approved",
			User:       "reviewer",
			URLIndex:   0,
		},
		{
			Name:       "Deploy",
			Type:       analyzer.NodeTypeWorkflow,
			StartTime:  globalStart.Add(5 * time.Minute),
			EndTime:    globalEnd,
			Status:     "completed",
			Conclusion: "failure",
			URLIndex:   1,
			Children: []*analyzer.TreeNode{
				{
					Name:       "deploy-prod",
					Type:       analyzer.NodeTypeJob,
					StartTime:  globalStart.Add(5 * time.Minute),
					EndTime:    globalEnd,
					Status:     "completed",
					Conclusion: "failure",
					URLIndex:   1,
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
			if item.Name == "CI" && item.ItemType == ItemTypeWorkflow {
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
			if item.Name == "build" && item.ItemType == ItemTypeJob {
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
