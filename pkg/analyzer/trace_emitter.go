package analyzer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TraceEmitter handles emitting OTel spans for workflow events.
type TraceEmitter struct {
	tracer trace.Tracer
}

func NewTraceEmitter(tracer trace.Tracer) *TraceEmitter {
	return &TraceEmitter{tracer: tracer}
}

func (e *TraceEmitter) EmitMarkers(ctx context.Context, data *RawData, urlIndex int) {
	// Emit OTel spans for review and merge events
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

		_, span := e.tracer.Start(ctx, name,
			trace.WithTimestamp(eventTime),
			trace.WithAttributes(
				attribute.String("type", "marker"),
				attribute.String("github.event_type", eventType),
				attribute.String("github.user", firstNonEmpty(event.Reviewer, event.MergedBy)),
				attribute.String("github.url", event.URL),
				attribute.String("github.event_id", fmt.Sprintf("%s-%s-%s-%s", event.Type, event.Time, firstNonEmpty(event.Reviewer, event.MergedBy), event.URL)),
				attribute.String("github.event_time", event.Time),
				attribute.Int("github.url_index", urlIndex),
			),
		)
		span.End(trace.WithTimestamp(eventTime.Add(time.Millisecond)))
	}

	if data.CommitTimeMs != nil {
		t := time.UnixMilli(*data.CommitTimeMs)
		_, span := e.tracer.Start(ctx, "Commit Created",
			trace.WithTimestamp(t),
			trace.WithAttributes(
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "commit"),
				attribute.Int("github.url_index", urlIndex),
			),
		)
		span.End(trace.WithTimestamp(t.Add(time.Millisecond)))
	}

	if data.CommitPushedAtMs != nil {
		t := time.UnixMilli(*data.CommitPushedAtMs)
		_, span := e.tracer.Start(ctx, "Commit Pushed",
			trace.WithTimestamp(t),
			trace.WithAttributes(
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "push"),
				attribute.Int("github.url_index", urlIndex),
			),
		)
		span.End(trace.WithTimestamp(t.Add(time.Millisecond)))
	}
}
