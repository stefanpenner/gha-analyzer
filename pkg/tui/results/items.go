package results

import (
	"fmt"
	"sort"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/enrichment"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

// ItemType represents the type of tree item
type ItemType int

const (
	ItemTypeURLGroup ItemType = iota
	ItemTypeWorkflow
	ItemTypeJob
	ItemTypeStep
	ItemTypeMarker
	ItemTypeActivityGroup
)

// String returns a human-readable string for the ItemType
func (t ItemType) String() string {
	switch t {
	case ItemTypeURLGroup:
		return "URLGroup"
	case ItemTypeWorkflow:
		return "Workflow"
	case ItemTypeJob:
		return "Job"
	case ItemTypeStep:
		return "Step"
	case ItemTypeMarker:
		return "Marker"
	case ItemTypeActivityGroup:
		return "ActivityGroup"
	default:
		return "Unknown"
	}
}

// TreeItem represents a single item in the tree view
type TreeItem struct {
	ID           string
	Name         string
	DisplayName  string
	StartTime    time.Time
	EndTime      time.Time
	IsBottleneck bool
	Depth        int
	HasChildren  bool
	IsExpanded   bool
	ItemType     ItemType
	ParentID     string
	Children     []*TreeItem
	Hints        enrichment.SpanHints // enrichment hints from source node
	sourceNode   *analyzer.TreeNode
}

// BuildTreeItems converts TreeNodes into TreeItems for the TUI.
// When multiple inputURLs are provided, roots are grouped under URL group items.
func BuildTreeItems(roots []*analyzer.TreeNode, expandedState map[string]bool, inputURLs []string) []*TreeItem {
	if len(inputURLs) <= 1 {
		// Single URL or no URLs: partition into workflows + Activity group
		return partitionAndGroup(roots, "", 0, expandedState)
	}

	// Multiple URLs: group roots by URLIndex
	grouped := make(map[int][]*analyzer.TreeNode)
	for _, root := range roots {
		grouped[root.URLIndex] = append(grouped[root.URLIndex], root)
	}

	var items []*TreeItem
	for urlIdx, inputURL := range inputURLs {
		children := grouped[urlIdx]

		// Build display name from parsed URL
		displayName := inputURL
		if parsed, err := utils.ParseGitHubURL(inputURL); err == nil {
			if parsed.Type == "pr" {
				displayName = fmt.Sprintf("PR #%s (%s/%s)", parsed.Identifier, parsed.Owner, parsed.Repo)
			} else {
				id := parsed.Identifier
				if len(id) > 8 {
					id = id[:8]
				}
				displayName = fmt.Sprintf("commit %s (%s/%s)", id, parsed.Owner, parsed.Repo)
			}
		}

		groupID := fmt.Sprintf("url-group/%d", urlIdx)

		// Calculate time bounds from children
		var earliest, latest time.Time
		for _, child := range children {
			if !child.StartTime.IsZero() && (earliest.IsZero() || child.StartTime.Before(earliest)) {
				earliest = child.StartTime
			}
			if !child.EndTime.IsZero() && (latest.IsZero() || child.EndTime.After(latest)) {
				latest = child.EndTime
			}
		}

		// Compute aggregate outcome
		outcome := aggregateOutcome(children)

		groupItem := &TreeItem{
			ID:          groupID,
			Name:        displayName,
			DisplayName: displayName,
			StartTime:   earliest,
			EndTime:     latest,
			Depth:       0,
			HasChildren: len(children) > 0,
			IsExpanded:  expandedState[groupID],
			ItemType:    ItemTypeURLGroup,
			Children:    []*TreeItem{},
			Hints: enrichment.SpanHints{
				URL:     inputURL,
				Outcome: outcome,
			},
		}

		groupItem.Children = partitionAndGroup(children, groupID, 1, expandedState)

		items = append(items, groupItem)
	}

	// Sort URL groups by start time
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartTime.Before(items[j].StartTime)
	})

	return items
}

// aggregateOutcome returns an aggregate outcome from child nodes using Hints.
func aggregateOutcome(nodes []*analyzer.TreeNode) string {
	hasFailure := false
	hasSuccess := false
	for _, n := range nodes {
		switch n.Hints.Outcome {
		case "failure":
			hasFailure = true
		case "success":
			hasSuccess = true
		}
	}
	if hasFailure {
		return "failure"
	}
	if hasSuccess {
		return "success"
	}
	return ""
}

// partitionAndGroup splits roots into workflow items and marker items,
// grouping all markers under a synthetic "Activity" node.
func partitionAndGroup(roots []*analyzer.TreeNode, parentID string, depth int, expandedState map[string]bool) []*TreeItem {
	var workflows []*analyzer.TreeNode
	var markers []*analyzer.TreeNode
	for _, root := range roots {
		if root.Hints.GroupKey == "activity" || root.Hints.IsMarker {
			markers = append(markers, root)
		} else {
			workflows = append(workflows, root)
		}
	}

	var items []*TreeItem

	// Activity group goes first (collapsed by default)
	if len(markers) > 0 {
		groupID := makeNodeID(parentID, "Activity", 0)

		var earliest, latest time.Time
		var children []*TreeItem
		for i, m := range markers {
			child := convertNode(m, groupID, i, depth+1, expandedState)
			children = append(children, child)
			if !m.StartTime.IsZero() && (earliest.IsZero() || m.StartTime.Before(earliest)) {
				earliest = m.StartTime
			}
			if !m.EndTime.IsZero() && (latest.IsZero() || m.EndTime.After(latest)) {
				latest = m.EndTime
			}
		}

		activityGroup := &TreeItem{
			ID:          groupID,
			Name:        "Activity",
			DisplayName: "Activity",
			StartTime:   earliest,
			EndTime:     latest,
			Depth:       depth,
			HasChildren: true,
			IsExpanded:  expandedState[groupID],
			ItemType:    ItemTypeActivityGroup,
			ParentID:    parentID,
			Children:    children,
		}
		items = append(items, activityGroup)
	}

	for i, wf := range workflows {
		items = append(items, convertNode(wf, parentID, i, depth, expandedState))
	}

	return items
}

func convertNode(node *analyzer.TreeNode, parentID string, index, depth int, expandedState map[string]bool) *TreeItem {
	id := makeNodeID(parentID, node.Name, index)

	itemType := itemTypeFromNode(node)

	item := &TreeItem{
		ID:          id,
		Name:        node.Name,
		DisplayName: node.Name,
		StartTime:   node.StartTime,
		EndTime:     node.EndTime,
		Depth:       depth,
		HasChildren: len(node.Children) > 0,
		IsExpanded:  expandedState[id],
		ItemType:    itemType,
		ParentID:    parentID,
		Children:    []*TreeItem{},
		Hints:       node.Hints,
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

// FilterVisibleItems filters already-flattened visible items to only include
// items whose ID is in matchIDs or ancestorIDs.
func FilterVisibleItems(items []TreeItem, matchIDs, ancestorIDs map[string]bool) []TreeItem {
	var result []TreeItem
	for _, item := range items {
		if matchIDs[item.ID] || ancestorIDs[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

// itemTypeFromNode derives ItemType from enrichment hints.
func itemTypeFromNode(node *analyzer.TreeNode) ItemType {
	hints := node.Hints
	if hints.IsMarker {
		return ItemTypeMarker
	}
	switch hints.Category {
	case "workflow":
		return ItemTypeWorkflow
	case "job":
		return ItemTypeJob
	case "step":
		return ItemTypeStep
	default:
		if hints.IsRoot {
			return ItemTypeWorkflow
		}
		if hints.IsLeaf {
			return ItemTypeStep
		}
		return ItemTypeJob
	}
}

func makeNodeID(parentID, name string, index int) string {
	if parentID == "" {
		return fmt.Sprintf("%s/%d", name, index)
	}
	return fmt.Sprintf("%s/%s/%d", parentID, name, index)
}

// BuildTreeFromSpans is a convenience wrapper that uses the default enricher.
func BuildTreeFromSpans(spans []trace.ReadOnlySpan, globalEarliest, globalLatest time.Time) []*analyzer.TreeNode {
	return analyzer.BuildTreeFromSpans(spans, globalEarliest, globalLatest, enrichment.DefaultEnricher())
}

// CountStats returns workflow and job counts from the tree
func CountStats(roots []*analyzer.TreeNode) (workflows, jobs int) {
	for _, root := range roots {
		if isRootNode(root) {
			workflows++
		}
		countJobs(root, &jobs)
	}
	return
}

// isRootNode checks if node is a root-level span.
func isRootNode(node *analyzer.TreeNode) bool {
	return node.Hints.IsRoot
}

// isJobNode checks if node is a job-level (mid-tier) span.
func isJobNode(node *analyzer.TreeNode) bool {
	return !node.Hints.IsRoot && !node.Hints.IsMarker && !node.Hints.IsLeaf
}

func countJobs(node *analyzer.TreeNode, count *int) {
	if isJobNode(node) {
		*count++
	}
	for _, child := range node.Children {
		countJobs(child, count)
	}
}
