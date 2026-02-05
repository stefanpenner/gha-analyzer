package results

import (
	"testing"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stretchr/testify/assert"
)

func TestItemTypeString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		itemType ItemType
		expected string
	}{
		{ItemTypeURLGroup, "URLGroup"},
		{ItemTypeWorkflow, "Workflow"},
		{ItemTypeJob, "Job"},
		{ItemTypeStep, "Step"},
		{ItemTypeMarker, "Marker"},
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
				Type:      analyzer.NodeTypeWorkflow,
				StartTime: now,
				EndTime:   now.Add(time.Minute),
				Status:    "completed",
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		assert.Equal(t, "CI", items[0].Name)
		assert.Equal(t, ItemTypeWorkflow, items[0].ItemType)
		assert.Equal(t, 0, items[0].Depth)
		assert.False(t, items[0].HasChildren)
	})

	t.Run("converts nested hierarchy", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{
				Name: "CI",
				Type: analyzer.NodeTypeWorkflow,
				Children: []*analyzer.TreeNode{
					{
						Name: "build",
						Type: analyzer.NodeTypeJob,
						Children: []*analyzer.TreeNode{
							{Name: "checkout", Type: analyzer.NodeTypeStep},
							{Name: "compile", Type: analyzer.NodeTypeStep},
						},
					},
				},
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Len(t, items, 1)
		assert.True(t, items[0].HasChildren)
		assert.Len(t, items[0].Children, 1)
		assert.Equal(t, "build", items[0].Children[0].Name)
		assert.Equal(t, ItemTypeJob, items[0].Children[0].ItemType)
		assert.Equal(t, 1, items[0].Children[0].Depth)
		assert.Len(t, items[0].Children[0].Children, 2)
	})

	t.Run("respects expanded state", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{Name: "CI", Type: analyzer.NodeTypeWorkflow},
		}
		expandedState := map[string]bool{"CI/0": true}

		items := BuildTreeItems(roots, expandedState, nil)

		assert.True(t, items[0].IsExpanded)
	})

	t.Run("preserves marker attributes", func(t *testing.T) {
		roots := []*analyzer.TreeNode{
			{
				Name:      "Approval",
				Type:      analyzer.NodeTypeMarker,
				User:      "reviewer",
				EventType: "approved",
			},
		}

		items := BuildTreeItems(roots, nil, nil)

		assert.Equal(t, "reviewer", items[0].User)
		assert.Equal(t, "approved", items[0].EventType)
		assert.Equal(t, ItemTypeMarker, items[0].ItemType)
	})
}

func TestFlattenVisibleItems(t *testing.T) {
	t.Parallel()

	t.Run("returns all root items when none expanded", func(t *testing.T) {
		items := []*TreeItem{
			{ID: "a", HasChildren: true, Children: []*TreeItem{{ID: "a/1"}}},
			{ID: "b", HasChildren: false},
		}

		result := FlattenVisibleItems(items, nil)

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

		result := FlattenVisibleItems(items, expanded)

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

		result := FlattenVisibleItems(items, expanded)

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

		result := FlattenVisibleItems(items, expanded)

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
				{Type: analyzer.NodeTypeWorkflow},
			},
			expectedWorkflows: 1,
			expectedJobs:      0,
		},
		{
			name: "workflow with jobs",
			roots: []*analyzer.TreeNode{
				{
					Type: analyzer.NodeTypeWorkflow,
					Children: []*analyzer.TreeNode{
						{Type: analyzer.NodeTypeJob},
						{Type: analyzer.NodeTypeJob},
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
					Type: analyzer.NodeTypeWorkflow,
					Children: []*analyzer.TreeNode{
						{Type: analyzer.NodeTypeJob},
					},
				},
				{
					Type: analyzer.NodeTypeWorkflow,
					Children: []*analyzer.TreeNode{
						{Type: analyzer.NodeTypeJob},
						{Type: analyzer.NodeTypeJob},
					},
				},
			},
			expectedWorkflows: 2,
			expectedJobs:      3,
		},
		{
			name: "markers not counted as workflows",
			roots: []*analyzer.TreeNode{
				{Type: analyzer.NodeTypeMarker},
				{Type: analyzer.NodeTypeWorkflow},
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
