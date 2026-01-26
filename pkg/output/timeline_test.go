package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestRenderOTelTimelineDeduplication(t *testing.T) {
	now := time.Now()
	
	// Helper to create a read-only span
	createSpan := func(name string, eventType string, eventID string, url string, startTime time.Time) sdktrace.ReadOnlySpan {
		return &mockReadOnlySpan{
			name:      name,
			startTime: startTime,
			endTime:   startTime.Add(time.Second),
			spanID:    trace.SpanID{1, 2, 3, 4, 5, 6, 7, byte(len(eventID))}, // Unique-ish spanID
			attrs: []attribute.KeyValue{
				attribute.String("type", "marker"),
				attribute.String("github.event_type", eventType),
				attribute.String("github.event_id", eventID),
				attribute.String("github.url", url),
				attribute.String("github.event_time", startTime.Format(time.RFC3339)),
			},
		}
	}

	t.Run("Deduplicates identical events with same eventID and time", func(t *testing.T) {
		eventTime := now.Truncate(time.Second)
		eventID := "review-123"
		
		span1 := createSpan("Review: APPROVED", "approved", eventID, "https://github.com/1", eventTime)
		span2 := createSpan("Review: APPROVED", "approved", eventID, "https://github.com/1", eventTime)
		
		var buf bytes.Buffer
		RenderOTelTimeline(&buf, []sdktrace.ReadOnlySpan{span1, span2}, time.Time{}, time.Time{})
		
		output := buf.String()
		// Should only contain one "Review: APPROVED"
		assert.Equal(t, 1, countOccurrences(output, "Review: APPROVED"))
	})

	t.Run("Preserves distinct events with same timestamp but different eventIDs", func(t *testing.T) {
		eventTime := now.Truncate(time.Second)
		
		// Same time, but different eventIDs (e.g. a review and a comment at the same time)
		span1 := createSpan("Review: APPROVED", "approved", "review-123-url1", "https://github.com/1#review", eventTime)
		span2 := createSpan("Comment", "comment", "comment-123-url2", "https://github.com/1#comment", eventTime)
		
		var buf bytes.Buffer
		RenderOTelTimeline(&buf, []sdktrace.ReadOnlySpan{span1, span2}, time.Time{}, time.Time{})
		
		output := buf.String()
		// We check for the presence of the labels which are now clickable links
		assert.Contains(t, output, "APPROVED")
		assert.Contains(t, output, "Comment")
	})

	t.Run("Sorts markers before workflows when timestamps are identical", func(t *testing.T) {
		eventTime := now.Truncate(time.Second)

		workflowSpan := &mockReadOnlySpan{
			name:      "Workflow: Test",
			startTime: eventTime,
			endTime:   eventTime.Add(time.Minute),
			spanID:    trace.SpanID{1, 1, 1, 1, 1, 1, 1, 1},
			attrs: []attribute.KeyValue{
				attribute.String("type", "workflow"),
			},
		}

		markerSpan := &mockReadOnlySpan{
			name:      "Commit Pushed",
			startTime: eventTime,
			endTime:   eventTime.Add(time.Millisecond),
			spanID:    trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2},
			attrs: []attribute.KeyValue{
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "push"),
			},
		}

		var buf bytes.Buffer
		// Provide spans in "wrong" order to test sorting
		RenderOTelTimeline(&buf, []sdktrace.ReadOnlySpan{workflowSpan, markerSpan}, time.Time{}, time.Time{})

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")

		// Find the lines containing the labels
		markerLineIdx := -1
		workflowLineIdx := -1
		for i, line := range lines {
			if strings.Contains(line, "Commit Pushed") {
				markerLineIdx = i
			}
			if strings.Contains(line, "Workflow: Test") {
				workflowLineIdx = i
			}
		}

		assert.True(t, markerLineIdx != -1, "Marker not found in output")
		assert.True(t, workflowLineIdx != -1, "Workflow not found in output")
		assert.True(t, markerLineIdx < workflowLineIdx, "Marker should appear before workflow in waterfall")
	})
}

type mockReadOnlySpan struct {
	sdktrace.ReadOnlySpan
	name      string
	startTime time.Time
	endTime   time.Time
	spanID    trace.SpanID
	attrs     []attribute.KeyValue
}

func (m *mockReadOnlySpan) Name() string                  { return m.name }
func (m *mockReadOnlySpan) StartTime() time.Time          { return m.startTime }
func (m *mockReadOnlySpan) EndTime() time.Time            { return m.endTime }
func (m *mockReadOnlySpan) Attributes() []attribute.KeyValue { return m.attrs }
func (m *mockReadOnlySpan) SpanContext() trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		SpanID: m.spanID,
	})
}
func (m *mockReadOnlySpan) Parent() trace.SpanContext      { return trace.SpanContext{} }
func (m *mockReadOnlySpan) Resource() *resource.Resource   { return nil }
func (m *mockReadOnlySpan) InstrumentationLibrary() instrumentation.Library { return instrumentation.Library{} }
func (m *mockReadOnlySpan) InstrumentationScope() instrumentation.Scope { return instrumentation.Scope{} }
func (m *mockReadOnlySpan) ChildSpanCount() int           { return 0 }
func (m *mockReadOnlySpan) Links() []sdktrace.Link        { return nil }
func (m *mockReadOnlySpan) Events() []sdktrace.Event      { return nil }
func (m *mockReadOnlySpan) Status() sdktrace.Status       { return sdktrace.Status{} }
func (m *mockReadOnlySpan) DroppedAttributesCount() int   { return 0 }
func (m *mockReadOnlySpan) DroppedEventsCount() int       { return 0 }
func (m *mockReadOnlySpan) DroppedLinksCount() int        { return 0 }
func (m *mockReadOnlySpan) ChildSpans() []sdktrace.ReadOnlySpan { return nil }

func countOccurrences(s, substr string) int {
	count := 0
	for {
		idx := bytes.Index([]byte(s), []byte(substr))
		if idx == -1 {
			break
		}
		count++
		s = s[idx+len(substr):]
	}
	return count
}
