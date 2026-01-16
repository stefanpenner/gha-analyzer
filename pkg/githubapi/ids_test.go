package githubapi

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestDeterministicIDs(t *testing.T) {
	t.Run("NewTraceID is deterministic", func(t *testing.T) {
		runID := int64(12345)
		attempt := int64(1)

		tid1 := NewTraceID(runID, attempt)
		tid2 := NewTraceID(runID, attempt)

		if tid1 != tid2 {
			t.Errorf("Expected same TraceID for same inputs, got %s and %s", tid1, tid2)
		}

		if !tid1.IsValid() {
			t.Error("Expected valid TraceID")
		}

		tid3 := NewTraceID(runID, 2)
		if tid1 == tid3 {
			t.Error("Expected different TraceID for different attempt")
		}
	})

	t.Run("NewSpanID is deterministic", func(t *testing.T) {
		jobID := int64(67890)

		sid1 := NewSpanID(jobID)
		sid2 := NewSpanID(jobID)

		if sid1 != sid2 {
			t.Errorf("Expected same SpanID for same inputs, got %s and %s", sid1, sid2)
		}

		if !sid1.IsValid() {
			t.Error("Expected valid SpanID")
		}

		sid3 := NewSpanID(67891)
		if sid1 == sid3 {
			t.Error("Expected different SpanID for different JobID")
		}
	})

	t.Run("NewSpanIDFromString is deterministic", func(t *testing.T) {
		name := "step-1"
		sid1 := NewSpanIDFromString(name)
		sid2 := NewSpanIDFromString(name)

		if sid1 != sid2 {
			t.Errorf("Expected same SpanID for same string, got %s and %s", sid1, sid2)
		}
	})
}

func TestGHIDGenerator(t *testing.T) {
	generator := GHIDGenerator{}

	t.Run("uses IDs from context when present", func(t *testing.T) {
		expectedTid := NewTraceID(1, 1)
		expectedSid := NewSpanID(1)
		ctx := ContextWithIDs(context.Background(), expectedTid, expectedSid)

		tid, sid := generator.NewIDs(ctx)

		if tid != expectedTid {
			t.Errorf("Expected TraceID %s, got %s", expectedTid, tid)
		}
		if sid != expectedSid {
			t.Errorf("Expected SpanID %s, got %s", expectedSid, sid)
		}
	})

	t.Run("falls back to random when context is empty", func(t *testing.T) {
		ctx := context.Background()
		tid1, sid1 := generator.NewIDs(ctx)
		tid2, sid2 := generator.NewIDs(ctx)

		if !tid1.IsValid() || !sid1.IsValid() {
			t.Error("Expected valid random IDs")
		}
		if tid1 == tid2 || sid1 == sid2 {
			t.Error("Expected different random IDs for each call")
		}
	})

	t.Run("NewSpanID uses context hint", func(t *testing.T) {
		expectedSid := NewSpanID(42)
		ctx := ContextWithIDs(context.Background(), trace.TraceID{}, expectedSid)

		sid := generator.NewSpanID(ctx, trace.TraceID{})

		if sid != expectedSid {
			t.Errorf("Expected SpanID %s, got %s", expectedSid, sid)
		}
	})
}
