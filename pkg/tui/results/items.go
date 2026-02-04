package results

import (
	"fmt"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"go.opentelemetry.io/otel/sdk/trace"
)

// ItemType represents the type of tree item
type ItemType int

const (
	ItemTypeWorkflow ItemType = iota
	ItemTypeJob
	ItemTypeStep
	ItemTypeMarker
)

// String returns a human-readable string for the ItemType
func (t ItemType) String() string {
	switch t {
	case ItemTypeWorkflow:
		return "Workflow"
	case ItemTypeJob:
		return "Job"
	case ItemTypeStep:
		return "Step"
	case ItemTypeMarker:
		return "Marker"
	default:
		return "Unknown"
	}
}

// TreeItem represents a single item in the tree view
type TreeItem struct {
	ID           string
	Name         string
	DisplayName  string
	URL          string
	StartTime    time.Time
	EndTime      time.Time
	Conclusion   string
	Status       string
	IsRequired   bool
	IsBottleneck bool
	Depth        int
	HasChildren  bool
	IsExpanded   bool
	ItemType     ItemType
	ParentID     string
	User         string // for markers
	EventType    string // for markers
	Children     []*TreeItem
	sourceNode   *analyzer.TreeNode
}

// BuildTreeItems converts TreeNodes into TreeItems for the TUI
func BuildTreeItems(roots []*analyzer.TreeNode, expandedState map[string]bool) []*TreeItem {
	var items []*TreeItem

	for i, root := range roots {
		item := convertNode(root, "", i, 0, expandedState)
		items = append(items, item)
	}

	return items
}

func convertNode(node *analyzer.TreeNode, parentID string, index, depth int, expandedState map[string]bool) *TreeItem {
	id := makeNodeID(parentID, node.Name, index)

	itemType := ItemTypeWorkflow
	switch node.Type {
	case analyzer.NodeTypeWorkflow:
		itemType = ItemTypeWorkflow
	case analyzer.NodeTypeJob:
		itemType = ItemTypeJob
	case analyzer.NodeTypeStep:
		itemType = ItemTypeStep
	case analyzer.NodeTypeMarker:
		itemType = ItemTypeMarker
	}

	item := &TreeItem{
		ID:          id,
		Name:        node.Name,
		DisplayName: node.Name,
		URL:         node.URL,
		StartTime:   node.StartTime,
		EndTime:     node.EndTime,
		Conclusion:  node.Conclusion,
		Status:      node.Status,
		IsRequired:  node.IsRequired,
		Depth:       depth,
		HasChildren: len(node.Children) > 0,
		IsExpanded:  expandedState[id],
		ItemType:    itemType,
		ParentID:    parentID,
		User:        node.User,
		EventType:   node.EventType,
		Children:    []*TreeItem{},
		sourceNode:  node,
	}

	// Convert children
	for i, child := range node.Children {
		childItem := convertNode(child, id, i, depth+1, expandedState)
		item.Children = append(item.Children, childItem)
	}

	return item
}

// FlattenVisibleItems returns a flat list of visible items based on expanded state
func FlattenVisibleItems(items []*TreeItem, expandedState map[string]bool) []TreeItem {
	var result []TreeItem

	var flatten func(items []*TreeItem)
	flatten = func(items []*TreeItem) {
		for _, item := range items {
			result = append(result, *item)
			if item.HasChildren && expandedState[item.ID] {
				flatten(item.Children)
			}
		}
	}

	flatten(items)
	return result
}

func makeNodeID(parentID, name string, index int) string {
	if parentID == "" {
		return fmt.Sprintf("%s/%d", name, index)
	}
	return fmt.Sprintf("%s/%s/%d", parentID, name, index)
}

// BuildTreeFromSpans is a convenience wrapper
func BuildTreeFromSpans(spans []trace.ReadOnlySpan, globalEarliest, globalLatest time.Time) []*analyzer.TreeNode {
	return analyzer.BuildTreeFromSpans(spans, globalEarliest, globalLatest)
}

// CountStats returns workflow and job counts from the tree
func CountStats(roots []*analyzer.TreeNode) (workflows, jobs int) {
	for _, root := range roots {
		if root.Type == analyzer.NodeTypeWorkflow {
			workflows++
		}
		countJobs(root, &jobs)
	}
	return
}

func countJobs(node *analyzer.TreeNode, count *int) {
	if node.Type == analyzer.NodeTypeJob {
		*count++
	}
	for _, child := range node.Children {
		countJobs(child, count)
	}
}
