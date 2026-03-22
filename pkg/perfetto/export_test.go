package perfetto

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/stefanpenner/otel-explorer/pkg/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// parseVarint reads a varint from a byte slice and returns value + bytes consumed.
func parseVarint(data []byte) (uint64, int) {
	val, n := binary.Uvarint(data)
	return val, n
}

// parseTag extracts field number and wire type from a tag.
func parseTag(tag uint64) (field uint32, wireType uint32) {
	return uint32(tag >> 3), uint32(tag & 0x7)
}

// skipField skips over a protobuf field value given its wire type.
func skipField(data []byte, wireType uint32) int {
	switch wireType {
	case 0: // varint
		_, n := parseVarint(data)
		return n
	case 1: // fixed64
		return 8
	case 2: // length-delimited
		length, n := parseVarint(data)
		return n + int(length)
	case 5: // fixed32
		return 4
	}
	return len(data) // consume rest on unknown
}

// extractSubmessages extracts all length-delimited fields with the given field number.
func extractSubmessages(data []byte, targetField uint32) [][]byte {
	var results [][]byte
	pos := 0
	for pos < len(data) {
		tag, n := parseVarint(data[pos:])
		if n <= 0 {
			break
		}
		pos += n
		field, wt := parseTag(tag)
		if field == targetField && wt == 2 {
			length, ln := parseVarint(data[pos:])
			pos += ln
			results = append(results, data[pos:pos+int(length)])
			pos += int(length)
		} else {
			consumed := skipField(data[pos:], wt)
			pos += consumed
		}
	}
	return results
}

// extractVarintField extracts a varint field value.
func extractVarintField(data []byte, targetField uint32) (uint64, bool) {
	pos := 0
	for pos < len(data) {
		tag, n := parseVarint(data[pos:])
		if n <= 0 {
			break
		}
		pos += n
		field, wt := parseTag(tag)
		if field == targetField && wt == 0 {
			val, vn := parseVarint(data[pos:])
			_ = vn
			return val, true
		}
		consumed := skipField(data[pos:], wt)
		pos += consumed
	}
	return 0, false
}

// extractStringField extracts a string field value.
func extractStringField(data []byte, targetField uint32) (string, bool) {
	pos := 0
	for pos < len(data) {
		tag, n := parseVarint(data[pos:])
		if n <= 0 {
			break
		}
		pos += n
		field, wt := parseTag(tag)
		if field == targetField && wt == 2 {
			length, ln := parseVarint(data[pos:])
			pos += ln
			return string(data[pos : pos+int(length)]), true
		}
		consumed := skipField(data[pos:], wt)
		pos += consumed
	}
	return "", false
}

func TestWriteTrace(t *testing.T) {
	globalStart := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

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
		StartTime: globalStart.Add(-5 * time.Minute),
		EndTime:   globalStart.Add(-5 * time.Minute),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "marker"),
			attribute.String("github.event_type", "approved"),
		},
	}

	tempFile := "test_trace.pftrace"
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

	s1 := span1.Snapshot()
	s2 := span2.Snapshot()

	err := WriteTrace(&buf, urlResults, combined, nil, globalStart.UnixMilli(), tempFile, false, []trace.ReadOnlySpan{s1, s2})
	require.NoError(t, err)

	// Read the protobuf output
	data, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.NotEmpty(t, data, "protobuf trace should not be empty")

	// Parse the Trace message: field 1 (repeated) = TracePacket
	packets := extractSubmessages(data, 1)
	assert.GreaterOrEqual(t, len(packets), 3, "should have at least descriptor + workflow begin/end + marker instant")

	// Check that we have TrackDescriptor packets (field 60 in TracePacket)
	var descriptorCount int
	var eventCount int
	var markerFound bool
	for _, pkt := range packets {
		descs := extractSubmessages(pkt, 60) // track_descriptor
		if len(descs) > 0 {
			descriptorCount++
		}
		trackEvents := extractSubmessages(pkt, 11) // track_event
		if len(trackEvents) > 0 {
			eventCount++
			for _, te := range trackEvents {
				// Check event type: field 9
				eventType, ok := extractVarintField(te, 9)
				if ok && eventType == typeInstant {
					// Check name: field 23
					name, nameOk := extractStringField(te, 23)
					if nameOk && name == "Review: APPROVED" {
						markerFound = true
					}
				}
			}
		}
	}

	assert.GreaterOrEqual(t, descriptorCount, 2, "should have at least 2 track descriptors (workflow process + marker tracks)")
	assert.GreaterOrEqual(t, eventCount, 3, "should have at least 3 events (workflow begin+end, marker instant)")
	assert.True(t, markerFound, "marker instant event 'Review: APPROVED' not found")
}

func TestWriteTraceWithLegacyEvents(t *testing.T) {
	globalStart := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	legacyEvents := []analyzer.TraceEvent{
		{
			Name: "Build",
			Ph:   "X",
			Ts:   5000000, // 5 seconds in microseconds
			Dur:  2000000, // 2 seconds
			Pid:  1,
			Tid:  1,
			Args: map[string]interface{}{"step": "compile"},
		},
	}

	tempFile := "test_legacy_trace.pftrace"
	defer os.Remove(tempFile)

	var buf bytes.Buffer
	err := WriteTrace(&buf, nil, analyzer.CombinedMetrics{}, legacyEvents, globalStart.UnixMilli(), tempFile, false, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	packets := extractSubmessages(data, 1)
	assert.GreaterOrEqual(t, len(packets), 3, "should have descriptor + begin + end for legacy event")
}

func TestProtobufRoundTrip(t *testing.T) {
	// Test that the protobuf encoder produces valid wire format
	var w protoWriter
	w.writeVarintField(1, 42)
	w.writeStringField(2, "hello")
	w.writeFixed64Field(3, 0xDEADBEEF)

	data := w.bytes()
	assert.NotEmpty(t, data)

	// Parse field 1 (varint)
	val, ok := extractVarintField(data, 1)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), val)

	// Parse field 2 (string)
	s, ok := extractStringField(data, 2)
	assert.True(t, ok)
	assert.Equal(t, "hello", s)
}
