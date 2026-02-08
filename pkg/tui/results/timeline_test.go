package results

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetCharWidth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		char     string
		expected int
	}{
		// Wide characters (emojis)
		{"💬", 2},
		{"📋", 2},
		{"⚙️", 2},
		{"❌", 2},
		{"🔒", 2},
		{"🔥", 2},
		// Single-width characters
		{"◆", 1},
		{"✓", 1},
		{"✗", 1},
		{"▲", 1},
		{"|", 1},
		{"↳", 1},
		{"◷", 1},
		{"○", 1},
		{"●", 1},
		{"▼", 1},
		{"▶", 1},
		{" ", 1},
		// Two-char combinations
		{"◆ ", 2},
		{"▲ ", 2},
		{"• ", 2},
		{"● ", 2},
	}

	for _, tc := range cases {
		t.Run(tc.char, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetCharWidth(tc.char))
		})
	}
}

func TestGetBarStyle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		item         TreeItem
		expectedChar string
	}{
		// Step styles
		{"step success", TreeItem{ItemType: ItemTypeStep, Conclusion: "success"}, "▒"},
		{"step failure", TreeItem{ItemType: ItemTypeStep, Conclusion: "failure"}, "▒"},
		{"step skipped", TreeItem{ItemType: ItemTypeStep, Conclusion: "skipped"}, "░"},
		{"step cancelled", TreeItem{ItemType: ItemTypeStep, Conclusion: "cancelled"}, "░"},
		{"step pending", TreeItem{ItemType: ItemTypeStep, Conclusion: ""}, "▒"},

		// Marker styles
		{"marker merged", TreeItem{ItemType: ItemTypeMarker, EventType: "merged"}, "◆"},
		{"marker approved", TreeItem{ItemType: ItemTypeMarker, EventType: "approved"}, "✓"},
		{"marker comment", TreeItem{ItemType: ItemTypeMarker, EventType: "comment"}, "●"},
		{"marker commented", TreeItem{ItemType: ItemTypeMarker, EventType: "commented"}, "●"},
		{"marker COMMENTED", TreeItem{ItemType: ItemTypeMarker, EventType: "COMMENTED"}, "●"},
		{"marker changes_requested", TreeItem{ItemType: ItemTypeMarker, EventType: "changes_requested"}, "✗"},
		{"marker unknown", TreeItem{ItemType: ItemTypeMarker, EventType: "unknown"}, "▲"},

		// Job/workflow styles
		{"job in_progress", TreeItem{ItemType: ItemTypeJob, Status: "in_progress"}, "▒"},
		{"job queued", TreeItem{ItemType: ItemTypeJob, Status: "queued"}, "▒"},
		{"job waiting", TreeItem{ItemType: ItemTypeJob, Status: "waiting"}, "▒"},
		{"job success", TreeItem{ItemType: ItemTypeJob, Conclusion: "success"}, "█"},
		{"job failure required", TreeItem{ItemType: ItemTypeJob, Conclusion: "failure", IsRequired: true}, "█"},
		{"job failure optional", TreeItem{ItemType: ItemTypeJob, Conclusion: "failure", IsRequired: false}, "░"},
		{"job skipped", TreeItem{ItemType: ItemTypeJob, Conclusion: "skipped"}, "░"},
		{"job cancelled", TreeItem{ItemType: ItemTypeJob, Conclusion: "cancelled"}, "░"},
		{"job unknown", TreeItem{ItemType: ItemTypeJob, Conclusion: ""}, "░"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			char, _ := getBarStyle(tc.item)
			assert.Equal(t, tc.expectedChar, char)
		})
	}
}

func TestRenderTimelineBar(t *testing.T) {
	t.Parallel()

	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Second)
	width := 20

	t.Run("returns spaces for invalid time range", func(t *testing.T) {
		// End before start
		result := RenderTimelineBar(TreeItem{}, now, now.Add(-time.Second), width, "")
		assert.Equal(t, strings.Repeat(" ", width), result)

		// Equal times
		result = RenderTimelineBar(TreeItem{}, now, now, width, "")
		assert.Equal(t, strings.Repeat(" ", width), result)
	})

	t.Run("returns spaces for zero width", func(t *testing.T) {
		result := RenderTimelineBar(TreeItem{}, globalStart, globalEnd, 0, "")
		assert.Equal(t, "", result)
	})

	t.Run("renders bar at start position", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeJob,
			StartTime:  globalStart,
			EndTime:    globalStart.Add(2 * time.Second),
			Conclusion: "success",
		}

		result := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		// Should have bar chars near the start
		// Result contains ANSI escape codes, so we check it doesn't start with all spaces
		assert.False(t, strings.HasPrefix(result, "     "), "Bar should start near beginning")
	})

	t.Run("renders bar at end position", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeJob,
			StartTime:  globalEnd.Add(-2 * time.Second),
			EndTime:    globalEnd,
			Conclusion: "success",
		}

		result := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		// Should have leading spaces (bar at end)
		assert.True(t, strings.HasPrefix(result, "    "), "Bar should be near end with leading spaces")
	})

	t.Run("clamps item outside global bounds", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeJob,
			StartTime:  globalStart.Add(-5 * time.Second), // Before global start
			EndTime:    globalEnd.Add(5 * time.Second),    // After global end
			Conclusion: "success",
		}

		result := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		// Should not panic and should produce output
		assert.NotEmpty(t, result)
	})

	t.Run("renders marker for zero-duration item", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeMarker,
			StartTime:  globalStart.Add(5 * time.Second),
			EndTime:    globalStart.Add(5 * time.Second), // Same as start
			EventType:  "approved",
		}

		result := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		assert.NotEmpty(t, result)
	})

	t.Run("renders pipe for zero-duration non-marker", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeJob,
			StartTime:  globalStart.Add(5 * time.Second),
			EndTime:    globalStart.Add(5 * time.Second),
			Conclusion: "success",
		}

		result := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		// Should contain | for zero-duration job
		assert.Contains(t, result, "|")
	})
}

func TestRenderTimelineBarSelected(t *testing.T) {
	t.Parallel()

	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Second)
	width := 20

	t.Run("returns spaces for invalid time range", func(t *testing.T) {
		result := RenderTimelineBarSelected(TreeItem{}, now, now.Add(-time.Second), width, "")
		assert.Equal(t, strings.Repeat(" ", width), result)
	})

	t.Run("renders bar for selected items", func(t *testing.T) {
		item := TreeItem{
			ItemType:   ItemTypeJob,
			StartTime:  globalStart,
			EndTime:    globalEnd,
			Conclusion: "success",
		}

		result := RenderTimelineBarSelected(item, globalStart, globalEnd, width, "")

		// Should contain the bar character
		assert.Contains(t, result, "█")
		// Note: ANSI color codes only appear when connected to TTY
		// The dimmed colors will be visible in actual TUI usage
	})
}

func TestRenderMarker(t *testing.T) {
	t.Parallel()

	width := 20

	t.Run("positions marker correctly", func(t *testing.T) {
		result := renderMarker("○", BarPendingStyle, 5, width, "", false)

		// Should have 5 leading spaces
		assert.True(t, strings.HasPrefix(result, "     "), "Should have 5 leading spaces")
	})

	t.Run("clamps negative position to 0", func(t *testing.T) {
		result := renderMarker("○", BarPendingStyle, -5, width, "", false)

		// Should start with marker, not spaces
		assert.True(t, strings.HasPrefix(result, "○"), "Should clamp to position 0")
	})

	t.Run("clamps position beyond width", func(t *testing.T) {
		result := renderMarker("○", BarPendingStyle, 100, width, "", false)

		// Should not panic and should be within width
		// The marker should be at the end
		trimmed := strings.TrimRight(result, " ")
		assert.True(t, strings.HasSuffix(trimmed, "○") || strings.Contains(result, "○"))
	})

	t.Run("adds hyperlink when URL provided", func(t *testing.T) {
		result := renderMarker("○", BarPendingStyle, 5, width, "https://example.com", false)

		// Should contain OSC 8 hyperlink sequence with id parameter
		assert.Contains(t, result, "\x1b]8;id=https://example.com;https://example.com\x07")
		assert.Contains(t, result, "\x1b]8;;\x07") // Closing sequence
	})
}

func TestTimelineHyperlink(t *testing.T) {
	t.Parallel()

	t.Run("returns text unchanged when URL empty", func(t *testing.T) {
		result := timelineHyperlink("", "text")
		assert.Equal(t, "text", result)
	})

	t.Run("wraps text in OSC 8 hyperlink", func(t *testing.T) {
		result := timelineHyperlink("https://example.com", "click me")

		// Should contain OSC 8 hyperlink sequence with id parameter
		assert.Contains(t, result, "\x1b]8;id=https://example.com;https://example.com\x07")
		assert.Contains(t, result, "click me")
		assert.Contains(t, result, "\x1b]8;;\x07")
	})

	t.Run("disables underline in hyperlink", func(t *testing.T) {
		result := timelineHyperlink("https://example.com", "text")

		// Should contain \x1b[24m to disable underline
		assert.Contains(t, result, "\x1b[24m")
	})
}

func TestRenderTimelineBarWithChildren(t *testing.T) {
	t.Parallel()

	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Second)
	width := 20

	t.Run("child markers appear at correct positions", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalStart.Add(4 * time.Second),
			Conclusion:  "success",
			HasChildren: true,
			Children: []*TreeItem{
				{
					StartTime:  globalStart.Add(6 * time.Second), // position 12
					EndTime:    globalStart.Add(7 * time.Second),
					Conclusion: "success",
				},
				{
					StartTime:  globalStart.Add(9 * time.Second), // position 18
					EndTime:    globalStart.Add(10 * time.Second),
					Conclusion: "failure",
				},
			},
		}

		result := RenderTimelineBarWithChildren(item, globalStart, globalEnd, width, "")

		// Child markers should be present as middle dot characters
		assert.Contains(t, result, "·", "Should contain child marker dots")
		assert.NotEmpty(t, result)
	})

	t.Run("parent bar overwrites child markers where they overlap", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalEnd, // full width bar
			Conclusion:  "success",
			HasChildren: true,
			Children: []*TreeItem{
				{
					StartTime:  globalStart.Add(5 * time.Second), // right in the middle of parent
					EndTime:    globalStart.Add(6 * time.Second),
					Conclusion: "success",
				},
			},
		}

		result := RenderTimelineBarWithChildren(item, globalStart, globalEnd, width, "")

		// Parent bar fills entire width, so child marker should be overwritten
		// Result should contain parent bar char but NOT the child dot
		assert.Contains(t, result, "█", "Parent bar should be present")
		assert.NotContains(t, result, "·", "Child marker should be overwritten by parent bar")
	})

	t.Run("no children produces same structure as normal RenderTimelineBar", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalStart.Add(5 * time.Second),
			Conclusion:  "success",
			HasChildren: true,
			Children:    []*TreeItem{}, // empty children
		}

		withChildren := RenderTimelineBarWithChildren(item, globalStart, globalEnd, width, "")
		normal := RenderTimelineBar(item, globalStart, globalEnd, width, "")

		// Both should contain the same bar character
		assert.Contains(t, withChildren, "█")
		assert.Contains(t, normal, "█")
	})

	t.Run("failure children use failure dim style", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalStart.Add(2 * time.Second),
			Conclusion:  "success",
			HasChildren: true,
			Children: []*TreeItem{
				{
					StartTime:  globalStart.Add(8 * time.Second), // well outside parent bar
					EndTime:    globalStart.Add(9 * time.Second),
					Conclusion: "failure",
				},
			},
		}

		result := RenderTimelineBarWithChildren(item, globalStart, globalEnd, width, "")

		// Should contain both parent bar and child marker
		assert.Contains(t, result, "█", "Parent bar should be present")
		assert.Contains(t, result, "·", "Child marker should be present")
	})

	t.Run("returns spaces for invalid time range", func(t *testing.T) {
		item := TreeItem{
			HasChildren: true,
			Children:    []*TreeItem{},
		}
		result := RenderTimelineBarWithChildren(item, now, now.Add(-time.Second), width, "")
		assert.Equal(t, strings.Repeat(" ", width), result)
	})

	t.Run("children with zero start time are skipped", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalStart.Add(2 * time.Second),
			Conclusion:  "success",
			HasChildren: true,
			Children: []*TreeItem{
				{
					// StartTime is zero
					EndTime:    globalStart.Add(5 * time.Second),
					Conclusion: "success",
				},
			},
		}

		result := RenderTimelineBarWithChildren(item, globalStart, globalEnd, width, "")

		// Should not contain child marker since start time is zero
		assert.NotContains(t, result, "·")
		assert.Contains(t, result, "█", "Parent bar should still be present")
	})
}

func TestRenderTimelineBarWithChildrenSelected(t *testing.T) {
	t.Parallel()

	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Second)
	width := 20

	t.Run("renders with selection styles", func(t *testing.T) {
		item := TreeItem{
			ItemType:    ItemTypeWorkflow,
			StartTime:   globalStart,
			EndTime:     globalStart.Add(4 * time.Second),
			Conclusion:  "success",
			HasChildren: true,
			Children: []*TreeItem{
				{
					StartTime:  globalStart.Add(7 * time.Second),
					EndTime:    globalStart.Add(8 * time.Second),
					Conclusion: "success",
				},
			},
		}

		result := RenderTimelineBarWithChildrenSelected(item, globalStart, globalEnd, width, "")

		// Should contain both bar and marker characters
		assert.Contains(t, result, "█", "Parent bar should be present")
		assert.Contains(t, result, "·", "Child marker should be present")
	})

	t.Run("returns selection bg for invalid time range", func(t *testing.T) {
		item := TreeItem{
			HasChildren: true,
			Children:    []*TreeItem{},
		}
		result := RenderTimelineBarWithChildrenSelected(item, now, now.Add(-time.Second), width, "")
		assert.NotEmpty(t, result)
	})
}

func TestComputeChildPositions(t *testing.T) {
	t.Parallel()

	now := time.Now()
	globalStart := now
	globalEnd := now.Add(10 * time.Second)
	width := 20

	t.Run("computes positions correctly", func(t *testing.T) {
		children := []*TreeItem{
			{StartTime: globalStart.Add(5 * time.Second), Conclusion: "success"},
			{StartTime: globalEnd, Conclusion: "failure"},
		}

		positions := computeChildPositions(children, globalStart, globalEnd, width, getChildMarkerStyle)

		assert.Len(t, positions, 2)
		assert.Equal(t, 10, positions[0].pos) // 5/10 * 20 = 10
		assert.Equal(t, 19, positions[1].pos) // clamped to width-1
	})

	t.Run("skips children with zero start time", func(t *testing.T) {
		children := []*TreeItem{
			{Conclusion: "success"}, // zero start time
		}

		positions := computeChildPositions(children, globalStart, globalEnd, width, getChildMarkerStyle)

		assert.Empty(t, positions)
	})

	t.Run("returns nil for zero width", func(t *testing.T) {
		children := []*TreeItem{
			{StartTime: globalStart.Add(5 * time.Second), Conclusion: "success"},
		}

		positions := computeChildPositions(children, globalStart, globalEnd, 0, getChildMarkerStyle)

		assert.Nil(t, positions)
	})

	t.Run("clamps positions before global start", func(t *testing.T) {
		children := []*TreeItem{
			{StartTime: globalStart.Add(-2 * time.Second), Conclusion: "success"},
		}

		positions := computeChildPositions(children, globalStart, globalEnd, width, getChildMarkerStyle)

		assert.Len(t, positions, 1)
		assert.Equal(t, 0, positions[0].pos)
	})
}

func TestMaxInt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b, expected int
	}{
		{1, 2, 2},
		{2, 1, 2},
		{0, 0, 0},
		{-1, 1, 1},
		{-5, -3, -3},
	}

	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tc.expected, maxInt(tc.a, tc.b))
		})
	}
}
