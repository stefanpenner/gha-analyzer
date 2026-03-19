package analyzer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stefanpenner/otel-analyzer/pkg/githubapi"
	"github.com/stefanpenner/otel-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TraceEmitter builds marker span stubs for workflow events and collects
// span events for the canonical OTel mapping.
type TraceEmitter struct {
	builder *SpanBuilder
	mu      sync.Mutex
	// Accumulated raw data per urlIndex for CollectEvents
	rawDataByURL map[int]*RawData
}

func NewTraceEmitter(builder *SpanBuilder) *TraceEmitter {
	return &TraceEmitter{
		builder:      builder,
		rawDataByURL: make(map[int]*RawData),
	}
}

// EmitMarkers creates marker spans (backward compat) AND stores data for CollectEvents.
func (e *TraceEmitter) EmitMarkers(data *RawData, urlIndex int) {
	e.mu.Lock()
	e.rawDataByURL[urlIndex] = data
	e.mu.Unlock()

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

// CollectEvents returns OTel span events for the workflow root span.
// These represent reviews, merges, commits, and pushes as proper span events
// rather than fake zero-duration spans.
func (e *TraceEmitter) CollectEvents(urlIndex int, traceID trace.TraceID) []sdktrace.Event {
	e.mu.Lock()
	data := e.rawDataByURL[urlIndex]
	e.mu.Unlock()

	if data == nil {
		return nil
	}

	var events []sdktrace.Event

	for _, event := range data.ReviewEvents {
		eventTime, _ := utils.ParseTime(event.Time)
		user := firstNonEmpty(event.Reviewer, event.MergedBy)

		switch event.Type {
		case "review":
			events = append(events, sdktrace.Event{
				Name: "github.review",
				Time: eventTime,
				Attributes: []attribute.KeyValue{
					attribute.String("github.review.state", event.State),
					attribute.String("github.user", user),
					attribute.String("github.url", event.URL),
				},
			})
		case "comment":
			events = append(events, sdktrace.Event{
				Name: "github.comment",
				Time: eventTime,
				Attributes: []attribute.KeyValue{
					attribute.String("github.user", user),
					attribute.String("github.url", event.URL),
				},
			})
		case "merged":
			attrs := []attribute.KeyValue{
				attribute.String("github.user", user),
				attribute.String("github.url", event.URL),
			}
			if event.PRNumber != 0 {
				attrs = append(attrs, attribute.Int("github.pr_number", event.PRNumber))
				attrs = append(attrs, attribute.String("github.pr_title", event.PRTitle))
			}
			events = append(events, sdktrace.Event{
				Name:       "github.merge",
				Time:       eventTime,
				Attributes: attrs,
			})
		}
	}

	if data.CommitTimeMs != nil {
		events = append(events, sdktrace.Event{
			Name: "github.commit",
			Time: time.UnixMilli(*data.CommitTimeMs),
			Attributes: []attribute.KeyValue{
				attribute.String("vcs.revision", data.HeadSHA),
			},
		})
	}

	if data.CommitPushedAtMs != nil {
		events = append(events, sdktrace.Event{
			Name: "github.push",
			Time: time.UnixMilli(*data.CommitPushedAtMs),
			Attributes: []attribute.KeyValue{
				attribute.String("vcs.revision", data.HeadSHA),
			},
		})
	}

	return events
}
