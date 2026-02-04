package analyzer

import (
	"sort"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
)

// NodeType represents the type of tree node
type NodeType string

const (
	NodeTypeWorkflow NodeType = "workflow"
	NodeTypeJob      NodeType = "job"
	NodeTypeStep     NodeType = "step"
	NodeTypeMarker   NodeType = "marker"
)

// TreeNode represents a node in the workflow/job/step hierarchy.
// This is a shared data structure used by both TUI and CLI rendering.
type TreeNode struct {
	Type        NodeType
	Name        string
	URL         string
	StartTime   time.Time
	EndTime     time.Time
	Conclusion  string
	Status      string
	IsRequired  bool
	User        string // for markers (reviewer, merged by, etc.)
	EventType   string // for markers (merged, approved, comment, etc.)
	Children    []*TreeNode
}

// Duration returns the duration of this node
func (n *TreeNode) Duration() time.Duration {
	return n.EndTime.Sub(n.StartTime)
}

// BuildTreeFromSpans constructs a hierarchy of TreeNodes from OTel spans.
// This filters to only include workflow, job, step, and marker spans.
func BuildTreeFromSpans(spans []trace.ReadOnlySpan, globalEarliest, globalLatest time.Time) []*TreeNode {
	if len(spans) == 0 {
		return nil
	}

	// Filter to only GHA-relevant spans
	type spanWithAttrs struct {
		span  trace.ReadOnlySpan
		attrs map[string]string
	}

	filtered := []spanWithAttrs{}
	seenMarkers := make(map[string]struct{})

	for _, s := range spans {
		attrs := make(map[string]string)
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.AsString()
		}

		spanType := attrs["type"]
		if spanType != "workflow" && spanType != "job" && spanType != "step" && spanType != "marker" {
			continue
		}

		// Filter by time bounds if provided
		if !globalEarliest.IsZero() && s.EndTime().Before(globalEarliest) {
			continue
		}
		if !globalLatest.IsZero() && s.StartTime().After(globalLatest) {
			continue
		}

		// Deduplicate markers using multiple attributes for robust deduplication
		if spanType == "marker" {
			eventID := attrs["github.event_id"]
			eventTime := attrs["github.event_time"]
			eventType := attrs["github.event_type"]
			user := attrs["github.user"]
			// Build a composite key from available attributes
			key := eventType + "-" + user + "-" + eventTime
			if eventID != "" {
				key = eventID + "-" + key
			}
			if _, seen := seenMarkers[key]; seen {
				continue
			}
			seenMarkers[key] = struct{}{}
		}

		filtered = append(filtered, spanWithAttrs{span: s, attrs: attrs})
	}

	if len(filtered) == 0 {
		return nil
	}

	// Build span ID to node mapping
	nodes := make(map[string]*TreeNode)
	spanMap := make(map[string]spanWithAttrs)

	for _, sa := range filtered {
		spanID := sa.span.SpanContext().SpanID().String()
		spanMap[spanID] = sa

		node := &TreeNode{
			Type:       NodeType(sa.attrs["type"]),
			Name:       sa.span.Name(),
			URL:        sa.attrs["github.url"],
			StartTime:  sa.span.StartTime(),
			EndTime:    sa.span.EndTime(),
			Conclusion: sa.attrs["github.conclusion"],
			Status:     sa.attrs["github.status"],
			IsRequired: sa.attrs["github.is_required"] == "true",
			User:       sa.attrs["github.user"],
			EventType:  sa.attrs["github.event_type"],
			Children:   []*TreeNode{},
		}
		nodes[spanID] = node
	}

	// Link children to parents
	var roots []*TreeNode
	for _, sa := range filtered {
		spanID := sa.span.SpanContext().SpanID().String()
		parentID := sa.span.Parent().SpanID().String()
		node := nodes[spanID]

		if parentID == "0000000000000000" {
			roots = append(roots, node)
		} else if parent, ok := nodes[parentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			// Parent not in this batch, treat as root
			roots = append(roots, node)
		}
	}

	// Sort all nodes by start time
	sortTreeNodes(roots)
	for _, node := range nodes {
		sortTreeNodes(node.Children)
	}

	return roots
}

// sortTreeNodes sorts nodes by start time, with markers first on ties
func sortTreeNodes(nodes []*TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartTime.Equal(nodes[j].StartTime) {
			// Tie-breaker: markers always come first
			if nodes[i].Type == NodeTypeMarker && nodes[j].Type != NodeTypeMarker {
				return true
			}
			if nodes[j].Type == NodeTypeMarker && nodes[i].Type != NodeTypeMarker {
				return false
			}
		}
		return nodes[i].StartTime.Before(nodes[j].StartTime)
	})
}

// FlattenTree flattens the tree into a list with depth information
type FlatNode struct {
	Node  *TreeNode
	Depth int
}

func FlattenTree(roots []*TreeNode) []FlatNode {
	var result []FlatNode
	var flatten func(nodes []*TreeNode, depth int)
	flatten = func(nodes []*TreeNode, depth int) {
		for _, node := range nodes {
			result = append(result, FlatNode{Node: node, Depth: depth})
			flatten(node.Children, depth+1)
		}
	}
	flatten(roots, 0)
	return result
}
