package analyzer

import (
	"sort"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/enrichment"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Summary represents the aggregated metrics from a set of spans.
type Summary struct {
	TotalRuns      int
	SuccessfulRuns int
	TotalJobs      int
	FailedJobs     int
	MaxConcurrency int
	// Enrichment metrics extracted from span attributes
	AvgQueueTimeMs float64
	MaxQueueTimeMs float64
	QueueCount     int
	RetriedRuns    int
	BillableMs     map[string]int64 // OS name → total ms (e.g. "ubuntu", "macos", "windows")
}

// CalculateSummary analyzes OTel spans to produce a high-level summary.
// It uses the provided enricher to classify spans as root/child and determine outcome.
func CalculateSummary(spans []trace.ReadOnlySpan, enricher enrichment.Enricher) Summary {
	s := Summary{
		BillableMs: make(map[string]int64),
	}

	var totalQueueMs float64

	for _, span := range spans {
		attrs := make(map[string]string)
		var attrInts map[string]int64
		for _, a := range span.Attributes() {
			key := string(a.Key)
			attrs[key] = a.Value.AsString()
			// Capture int64 attributes for billable/queue/retry
			if strings.HasPrefix(key, "billable.") || key == "queue_time_ms" || key == "github.run_attempt" {
				if attrInts == nil {
					attrInts = make(map[string]int64)
				}
				attrInts[key] = a.Value.AsInt64()
			}
		}

		isZeroDuration := span.EndTime().Before(span.StartTime()) || span.EndTime().Equal(span.StartTime())
		hints := enricher.Enrich(span.Name(), attrs, isZeroDuration)

		if hints.Category == "" {
			continue
		}

		if hints.IsRoot {
			s.TotalRuns++
			if hints.Outcome == "success" {
				s.SuccessfulRuns++
			}
			// Extract billable and retry data from workflow spans
			if attempt, ok := attrInts["github.run_attempt"]; ok && attempt > 1 {
				s.RetriedRuns++
			}
			for key, ms := range attrInts {
				if strings.HasPrefix(key, "billable.") && strings.HasSuffix(key, "_ms") {
					// "billable.ubuntu_ms" → "ubuntu"
					osName := strings.TrimSuffix(strings.TrimPrefix(key, "billable."), "_ms")
					s.BillableMs[osName] += ms
				}
			}
		} else if !hints.IsMarker && !hints.IsLeaf {
			// Non-root, non-marker, non-leaf = "job"-level span
			s.TotalJobs++
			if hints.Outcome == "failure" {
				s.FailedJobs++
			}
			// Extract queue time from job spans
			if qMs, ok := attrInts["queue_time_ms"]; ok && qMs > 0 {
				s.QueueCount++
				totalQueueMs += float64(qMs)
				if float64(qMs) > s.MaxQueueTimeMs {
					s.MaxQueueTimeMs = float64(qMs)
				}
			}
		}
	}

	if s.QueueCount > 0 {
		s.AvgQueueTimeMs = totalQueueMs / float64(s.QueueCount)
	}

	s.MaxConcurrency = CalculateConcurrency(spans, enricher)
	return s
}

// CalculateConcurrency calculates the maximum number of overlapping non-root,
// non-marker, non-leaf spans (i.e. "job"-level concurrency).
func CalculateConcurrency(spans []trace.ReadOnlySpan, enricher enrichment.Enricher) int {
	type event struct {
		ts    time.Time
		delta int // +1 for start, -1 for end
	}

	var events []event
	for _, s := range spans {
		attrs := make(map[string]string)
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.AsString()
		}

		isZeroDuration := s.EndTime().Before(s.StartTime()) || s.EndTime().Equal(s.StartTime())
		hints := enricher.Enrich(s.Name(), attrs, isZeroDuration)

		if hints.Category == "" || hints.IsRoot || hints.IsMarker || hints.IsLeaf {
			continue
		}

		events = append(events, event{s.StartTime(), 1})
		events = append(events, event{s.EndTime(), -1})
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].ts.Equal(events[j].ts) {
			return events[i].delta < events[j].delta // End before start if same time
		}
		return events[i].ts.Before(events[j].ts)
	})

	maxConcurrency := 0
	curr := 0
	for _, e := range events {
		curr += e.delta
		if curr > maxConcurrency {
			maxConcurrency = curr
		}
	}
	return maxConcurrency
}
