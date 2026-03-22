package analyzer

import (
	"testing"
	"time"

	"github.com/stefanpenner/otel-explorer/pkg/githubapi"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestWorkflowSpanSemconv verifies the workflow root span has CI/CD semconv attributes.
func TestWorkflowSpanSemconv(t *testing.T) {
	builder := &SpanBuilder{}
	run := githubapi.WorkflowRun{
		ID:         12345,
		RunAttempt: 1,
		Name:       "CI",
		Status:     "completed",
		Conclusion: "success",
		HeadSHA:    "abc123",
		HeadBranch: "main",
		Repository: githubapi.RepoRef{
			Owner: githubapi.RepoOwner{Login: "owner"},
			Name:  "repo",
		},
	}

	tid := githubapi.NewTraceID(run.ID, run.RunAttempt)
	wfSID := githubapi.NewSpanID(run.ID)
	wfSC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     wfSID,
		TraceFlags: trace.FlagsSampled,
	})

	now := time.Now()
	builder.Add(tracetest.SpanStub{
		Name:        "CI",
		SpanContext: wfSC,
		StartTime:   now,
		EndTime:     now.Add(5 * time.Minute),
		Attributes: []attribute.KeyValue{
			attribute.String("cicd.pipeline.name", "CI"),
			attribute.String("cicd.pipeline.run.id", "12345"),
			attribute.String("cicd.pipeline.run.result", "success"),
			attribute.String("vcs.repository.url.full", "https://github.com/owner/repo"),
			attribute.String("vcs.revision", "abc123"),
			attribute.String("vcs.ref.head.name", "main"),
			attribute.String("type", "workflow"),
		},
		Status: sdktrace.Status{Code: codes.Ok},
	})

	spans := builder.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]

	// Verify trace identity
	if span.SpanContext().TraceID() != tid {
		t.Errorf("trace ID mismatch")
	}
	if span.SpanContext().SpanID() != wfSID {
		t.Errorf("span ID mismatch")
	}

	// Verify it's a root span (no parent)
	if span.Parent().SpanID().IsValid() {
		t.Errorf("workflow span should be root (no parent)")
	}

	// Verify CI/CD semconv attributes
	assertAttr(t, span, "cicd.pipeline.name", "CI")
	assertAttr(t, span, "cicd.pipeline.run.id", "12345")
	assertAttr(t, span, "cicd.pipeline.run.result", "success")
	assertAttr(t, span, "vcs.repository.url.full", "https://github.com/owner/repo")
	assertAttr(t, span, "vcs.revision", "abc123")
	assertAttr(t, span, "vcs.ref.head.name", "main")

	// Verify backward compat
	assertAttr(t, span, "type", "workflow")

	// Verify status
	if span.Status().Code != codes.Ok {
		t.Errorf("expected status OK, got %v", span.Status().Code)
	}

	// Verify InstrumentationScope
	if span.InstrumentationScope().Name != instrumentationName {
		t.Errorf("expected scope %q, got %q", instrumentationName, span.InstrumentationScope().Name)
	}
}

// TestJobSpanSemconv verifies job spans have CI/CD semconv attributes.
func TestJobSpanSemconv(t *testing.T) {
	builder := &SpanBuilder{}
	tid := githubapi.NewTraceID(12345, 1)
	jobSID := githubapi.NewSpanID(67890)

	wfSC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     githubapi.NewSpanID(12345),
		TraceFlags: trace.FlagsSampled,
	})

	now := time.Now()
	builder.Add(tracetest.SpanStub{
		Name: "build",
		SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    tid,
			SpanID:     jobSID,
			TraceFlags: trace.FlagsSampled,
		}),
		Parent:    wfSC,
		StartTime: now,
		EndTime:   now.Add(3 * time.Minute),
		Attributes: []attribute.KeyValue{
			attribute.String("cicd.pipeline.task.name", "build"),
			attribute.String("cicd.pipeline.task.type", "build"),
			attribute.String("cicd.pipeline.task.run.result", "failure"),
			attribute.String("type", "job"),
		},
		Status: sdktrace.Status{Code: codes.Error, Description: "failure"},
	})

	spans := builder.Spans()
	span := spans[0]

	// Verify parent relationship
	if span.Parent().SpanID() != wfSC.SpanID() {
		t.Errorf("job parent should be workflow span")
	}
	if span.SpanContext().TraceID() != tid {
		t.Errorf("job should share workflow trace ID")
	}

	// Verify CI/CD semconv
	assertAttr(t, span, "cicd.pipeline.task.name", "build")
	assertAttr(t, span, "cicd.pipeline.task.type", "build")
	assertAttr(t, span, "cicd.pipeline.task.run.result", "failure")

	// Verify status
	if span.Status().Code != codes.Error {
		t.Errorf("expected status ERROR for failed job, got %v", span.Status().Code)
	}
}

// TestReviewEventsNotSpans verifies that review events are span events, not spans.
func TestReviewEventsNotSpans(t *testing.T) {
	builder := &SpanBuilder{}
	emitter := NewTraceEmitter(builder)

	reviewTime := "2024-06-15T10:30:00Z"
	data := &RawData{
		HeadSHA: "abc123",
		ReviewEvents: []ReviewEvent{
			{Type: "review", State: "APPROVED", Time: reviewTime, Reviewer: "alice", URL: "https://github.com/pr/1"},
			{Type: "merged", Time: reviewTime, MergedBy: "bob", URL: "https://github.com/pr/1", PRNumber: 42, PRTitle: "Add feature"},
		},
	}

	// EmitMarkers still creates legacy marker spans
	emitter.EmitMarkers(data, 0)

	// CollectEvents returns OTel span events for the workflow root span
	tid := githubapi.NewTraceID(12345, 1)
	events := emitter.CollectEvents(0, tid)

	if len(events) != 2 {
		t.Fatalf("expected 2 span events, got %d", len(events))
	}

	// First event: review
	if events[0].Name != "github.review" {
		t.Errorf("expected event name 'github.review', got %q", events[0].Name)
	}
	foundState := false
	for _, a := range events[0].Attributes {
		if string(a.Key) == "github.review.state" && a.Value.AsString() == "APPROVED" {
			foundState = true
		}
	}
	if !foundState {
		t.Error("expected github.review.state=APPROVED attribute on review event")
	}

	// Second event: merge
	if events[1].Name != "github.merge" {
		t.Errorf("expected event name 'github.merge', got %q", events[1].Name)
	}
}

// TestRetrySpanLink verifies retry attempts link to previous attempt.
func TestRetrySpanLink(t *testing.T) {
	run := githubapi.WorkflowRun{
		ID:         12345,
		RunAttempt: 2,
	}

	tid := githubapi.NewTraceID(run.ID, run.RunAttempt)
	prevTID := githubapi.NewTraceID(run.ID, 1)

	if tid == prevTID {
		t.Error("retry trace ID should differ from original")
	}

	// Verify the link construction
	prevWfSID := githubapi.NewSpanID(run.ID)
	link := sdktrace.Link{
		SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    prevTID,
			SpanID:     prevWfSID,
			TraceFlags: trace.FlagsSampled,
		}),
		Attributes: []attribute.KeyValue{
			attribute.String("link.type", "retry"),
			attribute.Int64("github.previous_attempt", 1),
		},
	}

	// Link should point to the previous attempt's trace
	if link.SpanContext.TraceID() != prevTID {
		t.Errorf("link should point to previous attempt trace ID")
	}
	if link.SpanContext.SpanID() != prevWfSID {
		t.Errorf("link should point to previous attempt workflow span")
	}
}

// TestGHConclusionMapping verifies the conclusion-to-OTel mapping.
func TestGHConclusionMapping(t *testing.T) {
	tests := []struct {
		conclusion string
		wantCode   codes.Code
		wantResult string
	}{
		{"success", codes.Ok, "success"},
		{"failure", codes.Error, "failure"},
		{"cancelled", codes.Ok, "cancelled"},
		{"skipped", codes.Ok, "skipped"},
		{"timed_out", codes.Error, "error"},
		{"", codes.Unset, ""},
	}

	for _, tt := range tests {
		t.Run(tt.conclusion, func(t *testing.T) {
			status := ghConclusionToStatus(tt.conclusion)
			if status.Code != tt.wantCode {
				t.Errorf("ghConclusionToStatus(%q) code = %v, want %v", tt.conclusion, status.Code, tt.wantCode)
			}
			result := ghConclusionToResult(tt.conclusion)
			if result != tt.wantResult {
				t.Errorf("ghConclusionToResult(%q) = %q, want %q", tt.conclusion, result, tt.wantResult)
			}
		})
	}
}

// TestDeterministicTraceIDs verifies trace ID determinism from run ID + attempt.
func TestDeterministicTraceIDs(t *testing.T) {
	// Same inputs → same trace ID
	tid1 := githubapi.NewTraceID(12345, 1)
	tid2 := githubapi.NewTraceID(12345, 1)
	if tid1 != tid2 {
		t.Error("same run ID + attempt should produce same trace ID")
	}

	// Different attempt → different trace ID
	tid3 := githubapi.NewTraceID(12345, 2)
	if tid1 == tid3 {
		t.Error("different attempt should produce different trace ID")
	}

	// Different run → different trace ID
	tid4 := githubapi.NewTraceID(99999, 1)
	if tid1 == tid4 {
		t.Error("different run ID should produce different trace ID")
	}
}

// assertAttr checks that a span has a string attribute with the expected value.
func assertAttr(t *testing.T, span sdktrace.ReadOnlySpan, key, want string) {
	t.Helper()
	for _, a := range span.Attributes() {
		if string(a.Key) == key {
			got := a.Value.AsString()
			if got != want {
				t.Errorf("attribute %q = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Errorf("attribute %q not found on span", key)
}
