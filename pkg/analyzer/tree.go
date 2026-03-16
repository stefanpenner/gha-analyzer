package analyzer

import (
	"sort"
	"time"

	"github.com/stefanpenner/otel-analyzer/pkg/enrichment"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TreeNode represents a node in the workflow/job/step hierarchy.
// This is a shared data structure used by both TUI and CLI rendering.
type TreeNode struct {
	Attrs     map[string]string    // raw span attributes
	Hints     enrichment.SpanHints // enrichment output
	Name      string
	StartTime time.Time
	EndTime   time.Time
	URLIndex  int // index of the input URL this node belongs to
	Children  []*TreeNode
}

// Duration returns the duration of this node
func (n *TreeNode) Duration() time.Duration {
	return n.EndTime.Sub(n.StartTime)
}

// BuildTreeFromSpans constructs a hierarchy of TreeNodes from OTel spans.
// Spans are filtered and enriched using the provided enricher.
func BuildTreeFromSpans(spans []trace.ReadOnlySpan, globalEarliest, globalLatest time.Time, enricher enrichment.Enricher) []*TreeNode {
	if len(spans) == 0 {
		return nil
	}

	type spanWithHints struct {
		span  trace.ReadOnlySpan
		attrs map[string]string
		hints enrichment.SpanHints
	}

	filtered := []spanWithHints{}
	seenDedup := make(map[string]struct{})

	for _, s := range spans {
		attrs := make(map[string]string)
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.AsString()
		}

		isZeroDuration := s.EndTime().Before(s.StartTime()) || s.EndTime().Equal(s.StartTime())
		hints := enricher.Enrich(s.Name(), attrs, isZeroDuration)

		// Skip spans the enricher doesn't recognize
		if hints.Category == "" {
			continue
		}

		// Filter by time bounds if provided
		if !globalEarliest.IsZero() && s.EndTime().Before(globalEarliest) {
			continue
		}
		if !globalLatest.IsZero() && s.StartTime().After(globalLatest) {
			continue
		}

		// Deduplicate using DedupKey from hints
		if hints.DedupKey != "" {
			if _, seen := seenDedup[hints.DedupKey]; seen {
				continue
			}
			seenDedup[hints.DedupKey] = struct{}{}
		}

		filtered = append(filtered, spanWithHints{span: s, attrs: attrs, hints: hints})
	}

	if len(filtered) == 0 {
		return nil
	}

	// Build span ID to node mapping
	nodes := make(map[string]*TreeNode)

	for _, sh := range filtered {
		spanID := sh.span.SpanContext().SpanID().String()

		urlIndex := 0
		for _, a := range sh.span.Attributes() {
			if string(a.Key) == "github.url_index" {
				urlIndex = int(a.Value.AsInt64())
				break
			}
		}

		node := &TreeNode{
			Attrs:     sh.attrs,
			Hints:     sh.hints,
			Name:      sh.span.Name(),
			StartTime: sh.span.StartTime(),
			EndTime:   sh.span.EndTime(),
			URLIndex:  urlIndex,
			Children:  []*TreeNode{},
		}
		nodes[spanID] = node
	}

	// Link children to parents
	var roots []*TreeNode
	for _, sh := range filtered {
		spanID := sh.span.SpanContext().SpanID().String()
		parentID := sh.span.Parent().SpanID().String()
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

// sortTreeNodes sorts nodes by start time, using SortPriority for tie-breaking
func sortTreeNodes(nodes []*TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartTime.Equal(nodes[j].StartTime) {
			// Tie-breaker: lower SortPriority first (markers have -1)
			if nodes[i].Hints.SortPriority != nodes[j].Hints.SortPriority {
				return nodes[i].Hints.SortPriority < nodes[j].Hints.SortPriority
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
