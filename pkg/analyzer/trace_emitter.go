package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TraceEmitter builds marker span stubs for workflow events.
type TraceEmitter struct {
	builder *SpanBuilder
}

func NewTraceEmitter(builder *SpanBuilder) *TraceEmitter {
	return &TraceEmitter{builder: builder}
}

func (e *TraceEmitter) EmitMarkers(data *RawData, urlIndex int) {
	for _, event := range data.ReviewEvents {
		eventTime, _ := utils.ParseTime(event.Time)
		name := "Marker"
		eventType := event.Type

		switch event.Type {
		case "review":
			name = fmt.Sprintf("Review: %s", event.State)
			eventType = strings.ToLower(event.State)
		case "comment":
			name = "Comment"
		case "merged":
			name = "Merged"
			if event.PRNumber != 0 {
				name = fmt.Sprintf("Merged PR #%d: %s", event.PRNumber, event.PRTitle)
			}
		}

		sid := githubapi.NewSpanIDFromString(fmt.Sprintf("marker-%s-%s-%s", event.Type, event.Time, firstNonEmpty(event.Reviewer, event.MergedBy)))
		e.builder.Add(tracetest.SpanStub{
			Name: name,
			SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
				SpanID:     sid,
				TraceFlags: trace.FlagsSampled,
			}),
			StartTime: eventTime,
			EndTime:   eventTime.Add(time.Millisecond),
			Attributes: []attribute.KeyValue{
				attribute.String("type", "marker"),
				attribute.String("github.event_type", eventType),
				attribute.String("github.user", firstNonEmpty(event.Reviewer, event.MergedBy)),
				attribute.String("github.url", event.URL),
				attribute.String("github.event_id", fmt.Sprintf("%s-%s-%s-%s", event.Type, event.Time, firstNonEmpty(event.Reviewer, event.MergedBy), event.URL)),
				attribute.String("github.event_time", event.Time),
				attribute.Int("github.url_index", urlIndex),
			},
		})
	}

	if data.CommitTimeMs != nil {
		t := time.UnixMilli(*data.CommitTimeMs)
		sid := githubapi.NewSpanIDFromString(fmt.Sprintf("commit-%d", *data.CommitTimeMs))
		e.builder.Add(tracetest.SpanStub{
			Name: "Commit Created",
			SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
				SpanID:     sid,
				TraceFlags: trace.FlagsSampled,
			}),
			StartTime: t,
			EndTime:   t.Add(time.Millisecond),
			Attributes: []attribute.KeyValue{
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "commit"),
				attribute.Int("github.url_index", urlIndex),
			},
		})
	}

	if data.CommitPushedAtMs != nil {
		t := time.UnixMilli(*data.CommitPushedAtMs)
		sid := githubapi.NewSpanIDFromString(fmt.Sprintf("push-%d", *data.CommitPushedAtMs))
		e.builder.Add(tracetest.SpanStub{
			Name: "Commit Pushed",
			SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
				SpanID:     sid,
				TraceFlags: trace.FlagsSampled,
			}),
			StartTime: t,
			EndTime:   t.Add(time.Millisecond),
			Attributes: []attribute.KeyValue{
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "push"),
				attribute.Int("github.url_index", urlIndex),
			},
		})
	}
}
