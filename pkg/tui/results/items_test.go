package results

import (
	"testing"
	"time"

	"github.com/stefanpenner/otel-explorer/pkg/analyzer"
	"github.com/stefanpenner/otel-explorer/pkg/enrichment"
	"github.com/stretchr/testify/assert"
)

func TestItemTypeString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		itemType ItemType
		expected string
	}{
		{ItemTypeURLGroup, "URLGroup"},
		{ItemTypeRoot, "Root"},
		{ItemTypeIntermediate, "Intermediate"},
		{ItemTypeLeaf, "Leaf"},
		{ItemTypeMarker, "Marker"},
		{ItemTypeActivityGroup, "ActivityGroup"},
		{ItemType(99), "Unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.itemType.String())
		})
	}
}

func TestMakeNodeID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		parentID string
		nodeName string
		index    int
		expected string
	}{
		{"root node", "", "workflow", 0, "workflow/0"},
		{"root node index 1", "", "workflow", 1, "workflow/1"},
		{"child node", "workflow/0", "job", 0, "workflow/0/job/0"},
		{"nested child", "workflow/0/job/0", "step", 2, "workflow/0/job/0/step/2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, makeNodeID(tc.parentID, tc.nodeName, tc.index))
		})
	}
}

func TestBuildTreeItems(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("converts single workflow node", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{
				Name:      "CI",
				Hints:     enrichment.SpanHints{Category: "workflow", IsRoot: true, Outcome: "success", Color: "green", BarChar: "█"},
				StartTime: now,
				EndTime:   now.Add(time.Minute),
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		// Single URL group wrapping the workflow
		assert.Len(t, items, 1)
		assert.Equal(t, ItemTypeURLGroup, items[0].ItemType)
		children := items[0].Children
		assert.Len(t, children, 1)
		assert.Equal(t, "CI", children[0].Name)
		assert.Equal(t, ItemTypeRoot, children[0].ItemType)
		assert.Equal(t, 1, children[0].Depth)
		assert.False(t, children[0].HasChildren)
	})

	t.Run("converts nested hierarchy", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{
				Name:  "CI",
				Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true},
				Children: []*analyzer.TreeNode{
					{
						Name:  "build",
						Hints: enrichment.SpanHints{Category: "job"},
						Children: []*analyzer.TreeNode{
							{Name: "checkout", Hints: enrichment.SpanHints{Category: "step", IsLeaf: true}},
							{Name: "compile", Hints: enrichment.SpanHints{Category: "step", IsLeaf: true}},
						},
					},
				},
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		ci := items[0].Children[0]
		assert.True(t, ci.HasChildren)
		assert.Equal(t, "build", ci.Children[0].Name)
		assert.Equal(t, ItemTypeIntermediate, ci.Children[0].ItemType)
		assert.Equal(t, 2, ci.Children[0].Depth)
		assert.Len(t, ci.Children[0].Children, 2)
	})

	t.Run("respects expanded state", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "CI", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}},
		}
		expandedState := map[string]bool{"url-group/0/CI/0": true}

		items := BuildTreeItems(roots, expandedState, nil)

		assert.True(t, items[0].Children[0].IsExpanded)
	})

	t.Run("preserves marker attributes under Activity group", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{
				Name:  "Approval",
				Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", User: "reviewer", EventType: "approved"},
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		children := items[0].Children
		assert.Len(t, children, 1)
		actGroup := children[0]
		assert.Equal(t, ItemTypeActivityGroup, actGroup.ItemType)
		assert.Equal(t, "Activity", actGroup.Name)
		assert.True(t, actGroup.HasChildren)
		assert.Len(t, actGroup.Children, 1)

		marker := actGroup.Children[0]
		assert.Equal(t, "reviewer", marker.Hints.User)
		assert.Equal(t, "approved", marker.Hints.EventType)
		assert.Equal(t, ItemTypeMarker, marker.ItemType)
	})
}

func TestBuildTreeItemsPartitioning(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("workflows at root, markers grouped under Activity", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "Tests", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}, StartTime: now, EndTime: now.Add(time.Minute)},
			{Name: "approved", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", User: "bob", EventType: "approved"}, StartTime: now.Add(2 * time.Minute), EndTime: now.Add(2 * time.Minute)},
			{Name: "Deploy", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}, StartTime: now.Add(time.Minute), EndTime: now.Add(3 * time.Minute)},
			{Name: "merged", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", User: "alice", EventType: "merged"}, StartTime: now.Add(4 * time.Minute), EndTime: now.Add(4 * time.Minute)},
		}

		items := BuildTreeItems(roots, nil, nil)

		// Single-URL: one URL group wrapper, children contain the partitioned items
		assert.Len(t, items, 1)
		assert.Equal(t, ItemTypeURLGroup, items[0].ItemType)
		children := items[0].Children
		assert.Len(t, children, 3)
		assert.Equal(t, ItemTypeActivityGroup, children[0].ItemType)
		assert.Equal(t, "Activity", children[0].Name)
		assert.Equal(t, ItemTypeRoot, children[1].ItemType)
		assert.Equal(t, "Tests", children[1].Name)
		assert.Equal(t, ItemTypeRoot, children[2].ItemType)
		assert.Equal(t, "Deploy", children[2].Name)

		assert.Len(t, children[0].Children, 2)
		assert.Equal(t, ItemTypeMarker, children[0].Children[0].ItemType)
		assert.Equal(t, "bob", children[0].Children[0].Hints.User)
		assert.Equal(t, ItemTypeMarker, children[0].Children[1].ItemType)
		assert.Equal(t, "alice", children[0].Children[1].Hints.User)
	})

	t.Run("no markers means no Activity group", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "Tests", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}},
			{Name: "Deploy", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		children := items[0].Children
		assert.Len(t, children, 2)
		for _, item := range children {
			assert.Equal(t, ItemTypeRoot, item.ItemType)
		}
	})

	t.Run("only markers produces only Activity group", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "comment", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", User: "carol", EventType: "comment"}},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		children := items[0].Children
		assert.Len(t, children, 1)
		assert.Equal(t, ItemTypeActivityGroup, children[0].ItemType)
		assert.Len(t, children[0].Children, 1)
	})

	t.Run("Activity group aggregates time bounds", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "comment", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity"}, StartTime: now.Add(2 * time.Minute), EndTime: now.Add(2 * time.Minute)},
			{Name: "merged", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity"}, StartTime: now.Add(5 * time.Minute), EndTime: now.Add(5 * time.Minute)},
		}

		items := BuildTreeItems(roots, nil, nil)

		children := items[0].Children
		assert.Equal(t, now.Add(2*time.Minute), children[0].StartTime)
		assert.Equal(t, now.Add(5*time.Minute), children[0].EndTime)
	})

	t.Run("multi-URL mode groups markers under Activity per URL group", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "Tests", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}, URLIndex: 0},
			{Name: "approved", Hints: enrichment.SpanHints{Category: "marker", IsMarker: true, GroupKey: "activity", User: "bob"}, URLIndex: 0, StartTime: now, EndTime: now},
			{Name: "Deploy", Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}, URLIndex: 1, StartTime: now, EndTime: now},
		}

		items := BuildTreeItems(roots, nil, []string{
			"https://github.com/owner/repo/pull/1",
			"https://github.com/owner/repo/pull/2",
		})

		assert.Len(t, items, 2)

		// Find PR #1 group
		var pr1Group *TreeItem
		for _, item := range items {
			if item.Hints.URL == "https://github.com/owner/repo/pull/1" {
				pr1Group = item
				break
			}
		}
		if assert.NotNil(t, pr1Group) {
			assert.Len(t, pr1Group.Children, 2)
			assert.Equal(t, ItemTypeActivityGroup, pr1Group.Children[0].ItemType)
			assert.Len(t, pr1Group.Children[0].Children, 1)
			assert.Equal(t, ItemTypeRoot, pr1Group.Children[1].ItemType)
		}
	})
}

func TestFlattenVisibleItems(t *testing.T) {
	t.Parallel()

	t.Run("returns all root items when none expanded", func(t *testing.T) {
		items := []*TreeItem{
			{ID: "a", HasChildren: true, Children: []*TreeItem{{ID: "a/1"}}},
			{ID: "b", HasChildren: false},
		}

		result := FlattenVisibleItems(items, nil, SortByStartTime)

		assert.Len(t, result, 2)
		assert.Equal(t, "a", result[0].ID)
		assert.Equal(t, "b", result[1].ID)
	})

	t.Run("includes children when parent expanded", func(t *testing.T) {
		items := []*TreeItem{
			{
				ID:          "a",
				HasChildren: true,
				Children: []*TreeItem{
					{ID: "a/1", HasChildren: false},
					{ID: "a/2", HasChildren: false},
				},
			},
		}
		expanded := map[string]bool{"a": true}

		result := FlattenVisibleItems(items, expanded, SortByStartTime)

		assert.Len(t, result, 3)
		assert.Equal(t, "a", result[0].ID)
		assert.Equal(t, "a/1", result[1].ID)
		assert.Equal(t, "a/2", result[2].ID)
	})

	t.Run("nested expansion", func(t *testing.T) {
		items := []*TreeItem{
			{
				ID:          "workflow",
				HasChildren: true,
				Children: []*TreeItem{
					{
						ID:          "workflow/job",
						HasChildren: true,
						Children: []*TreeItem{
							{ID: "workflow/job/step1"},
							{ID: "workflow/job/step2"},
						},
					},
				},
			},
		}
		expanded := map[string]bool{"workflow": true, "workflow/job": true}

		result := FlattenVisibleItems(items, expanded, SortByStartTime)

		assert.Len(t, result, 4)
	})

	t.Run("partial expansion", func(t *testing.T) {
		items := []*TreeItem{
			{
				ID:          "workflow",
				HasChildren: true,
				Children: []*TreeItem{
					{
						ID:          "workflow/job",
						HasChildren: true,
						Children:    []*TreeItem{{ID: "workflow/job/step"}},
					},
				},
			},
		}
		// Only expand workflow, not the job
		expanded := map[string]bool{"workflow": true}

		result := FlattenVisibleItems(items, expanded, SortByStartTime)

		assert.Len(t, result, 2)
		assert.Equal(t, "workflow", result[0].ID)
		assert.Equal(t, "workflow/job", result[1].ID)
	})
}

func TestCountStats(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		roots             []*analyzer.TreeNode
		expectedWorkflows int
		expectedJobs      int
	}{
		{
			name:              "empty tree",
			roots:             nil,
			expectedWorkflows: 0,
			expectedJobs:      0,
		},
		{
			name: "single workflow no jobs",
			roots: []*analyzer.TreeNode{
				{Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}},
			},
			expectedWorkflows: 1,
			expectedJobs:      0,
		},
		{
			name: "workflow with jobs",
			roots: []*analyzer.TreeNode{
				{
					Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true},
					Children: []*analyzer.TreeNode{
						{Hints: enrichment.SpanHints{Category: "job"}},
						{Hints: enrichment.SpanHints{Category: "job"}},
					},
				},
			},
			expectedWorkflows: 1,
			expectedJobs:      2,
		},
		{
			name: "multiple workflows with nested jobs",
			roots: []*analyzer.TreeNode{
				{
					Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true},
					Children: []*analyzer.TreeNode{
						{Hints: enrichment.SpanHints{Category: "job"}},
					},
				},
				{
					Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true},
					Children: []*analyzer.TreeNode{
						{Hints: enrichment.SpanHints{Category: "job"}},
						{Hints: enrichment.SpanHints{Category: "job"}},
					},
				},
			},
			expectedWorkflows: 2,
			expectedJobs:      3,
		},
		{
			name: "markers not counted as workflows",
			roots: []*analyzer.TreeNode{
				{Hints: enrichment.SpanHints{Category: "marker", IsMarker: true}},
				{Hints: enrichment.SpanHints{Category: "workflow", IsRoot: true}},
			},
			expectedWorkflows: 1,
			expectedJobs:      0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workflows, jobs := CountStats(tc.roots)
			assert.Equal(t, tc.expectedWorkflows, workflows)
			assert.Equal(t, tc.expectedJobs, jobs)
		})
	}
}

func TestSortMode(t *testing.T) {
	t.Parallel()

	t.Run("String returns expected labels", func(t *testing.T) {
		assert.Equal(t, "start", SortByStartTime.String())
		assert.Equal(t, "duration↓", SortByDurationDesc.String())
		assert.Equal(t, "duration↑", SortByDurationAsc.String())
	})

	t.Run("Next cycles through modes", func(t *testing.T) {
		assert.Equal(t, SortByDurationDesc, SortByStartTime.Next())
		assert.Equal(t, SortByDurationAsc, SortByDurationDesc.Next())
		assert.Equal(t, SortByStartTime, SortByDurationAsc.Next())
	})
}

func TestFlattenVisibleItemsWithSort(t *testing.T) {
	t.Parallel()

	now := time.Now()
	items := []*TreeItem{
		{ID: "short", StartTime: now, EndTime: now.Add(1 * time.Second)},
		{ID: "long", StartTime: now, EndTime: now.Add(10 * time.Second)},
		{ID: "medium", StartTime: now, EndTime: now.Add(5 * time.Second)},
	}

	t.Run("SortByStartTime preserves order", func(t *testing.T) {
		result := FlattenVisibleItems(items, nil, SortByStartTime)
		assert.Equal(t, "short", result[0].ID)
		assert.Equal(t, "long", result[1].ID)
		assert.Equal(t, "medium", result[2].ID)
	})

	t.Run("SortByDurationDesc sorts longest first", func(t *testing.T) {
		result := FlattenVisibleItems(items, nil, SortByDurationDesc)
		assert.Equal(t, "long", result[0].ID)
		assert.Equal(t, "medium", result[1].ID)
		assert.Equal(t, "short", result[2].ID)
	})

	t.Run("SortByDurationAsc sorts shortest first", func(t *testing.T) {
		result := FlattenVisibleItems(items, nil, SortByDurationAsc)
		assert.Equal(t, "short", result[0].ID)
		assert.Equal(t, "medium", result[1].ID)
		assert.Equal(t, "long", result[2].ID)
	})
}
