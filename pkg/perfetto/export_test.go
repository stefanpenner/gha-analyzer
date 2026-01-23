package perfetto

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWriteTrace(t *testing.T) {
	// Create some mock OTel spans
	globalStart := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	
	// Use tracetest to create ReadOnlySpans
	span1 := &tracetest.SpanStub{
		Name:      "Workflow Run",
		StartTime: globalStart,
		EndTime:   globalStart.Add(10 * time.Minute),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "workflow"),
			attribute.Int64("github.run_id", 12345),
		},
	}
	
	span2 := &tracetest.SpanStub{
		Name:      "Review: APPROVED",
		StartTime: globalStart.Add(-5 * time.Minute), // Marker BEFORE workflow
		EndTime:   globalStart.Add(-5 * time.Minute),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "marker"),
			attribute.String("github.event_type", "approved"),
		},
	}

	tempFile := "test_trace.json"
	defer os.Remove(tempFile)

	var buf bytes.Buffer
	urlResults := []analyzer.URLResult{
		{
			URLIndex:     0,
			EarliestTime: globalStart.UnixMilli(),
			DisplayName:  "PR 1",
		},
	}
	combined := analyzer.CombinedMetrics{
		TotalRuns:   1,
		SuccessRate: "100",
	}

	// Snapshot() provides the ReadOnlySpan interface
	s1 := span1.Snapshot()
	s2 := span2.Snapshot()

	err := WriteTrace(&buf, urlResults, combined, nil, globalStart.UnixMilli(), tempFile, false, []trace.ReadOnlySpan{s1, s2})
	require.NoError(t, err)

	// Verify the saved file
	data, err := os.ReadFile(tempFile)
	require.NoError(t, err)

	var output map[string]interface{}
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	events, ok := output["traceEvents"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, events)

	// Verify marker is not clamped and doesn't have negative timestamp
	var markerFound bool
	for _, e := range events {
		ev := e.(map[string]interface{})
		name := ev["name"].(string)
		if name == "Review: APPROVED" {
			markerFound = true
			tsVal, ok := ev["ts"]
			require.True(t, ok, "ts missing for marker")
			ts := tsVal.(float64)
			
			// Global earliest was globalStart. -5m marker makes it globalStart - 5m.
			// So marker starts at 0.
			assert.Equal(t, 0.0, ts)
			
			ph, _ := ev["ph"].(string)
			assert.Equal(t, "i", ph)
			
			pid, _ := ev["pid"].(float64)
			assert.Equal(t, 999.0, pid) // All markers should be in process 999
		}
	}
	assert.True(t, markerFound, "Marker not found in trace output")
}
