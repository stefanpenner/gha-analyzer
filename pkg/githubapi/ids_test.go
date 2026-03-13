package githubapi

import (
	"testing"
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
