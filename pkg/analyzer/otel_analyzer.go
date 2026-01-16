package analyzer

import (
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
)

var tracer = otel.Tracer("analyzer")

// Summary represents the aggregated metrics from a set of spans.
type Summary struct {
	TotalRuns      int
	SuccessfulRuns int
	TotalJobs      int
	FailedJobs     int
	MaxConcurrency int
}

// CalculateSummary analyzes OTel spans to produce a high-level summary.
func CalculateSummary(spans []trace.ReadOnlySpan) Summary {
	s := Summary{}
	var workflowSpans []trace.ReadOnlySpan
	
	for _, span := range spans {
		attrs := make(map[string]attribute.Value)
		for _, a := range span.Attributes() {
			attrs[string(a.Key)] = a.Value
		}

		if attrs["type"].AsString() == "workflow" {
			s.TotalRuns++
			if attrs["github.conclusion"].AsString() == "success" {
				s.SuccessfulRuns++
			}
			workflowSpans = append(workflowSpans, span)
		} else if attrs["type"].AsString() == "job" {
			s.TotalJobs++
			if attrs["github.conclusion"].AsString() == "failure" {
				s.FailedJobs++
			}
		}
	}
	
	s.MaxConcurrency = CalculateConcurrency(spans, "job")
	return s
}

// CalculateConcurrency calculates the maximum number of overlapping spans of a certain type.
func CalculateConcurrency(spans []trace.ReadOnlySpan, spanType string) int {
	type event struct {
		ts   time.Time
		type_ int // +1 for start, -1 for end
	}
	
	var events []event
	for _, s := range spans {
		// Filter by type if provided
		if spanType != "" {
			found := false
			for _, attr := range s.Attributes() {
				if attr.Key == "type" && attr.Value.AsString() == spanType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		events = append(events, event{s.StartTime(), 1})
		events = append(events, event{s.EndTime(), -1})
	}
	
	sort.Slice(events, func(i, j int) bool {
		if events[i].ts.Equal(events[j].ts) {
			return events[i].type_ < events[j].type_ // End before start if same time
		}
		return events[i].ts.Before(events[j].ts)
	})
	
	max := 0
	curr := 0
	for _, e := range events {
		curr += e.type_
		if curr > max {
			max = curr
		}
	}
	return max
}
