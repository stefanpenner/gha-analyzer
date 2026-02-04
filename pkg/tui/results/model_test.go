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
