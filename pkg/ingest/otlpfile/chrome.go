package otlpfile

// Chrome Tracing format parser.
//
// Supports the Trace Event Format used by Chrome DevTools, Perfetto,
// and other profiling tools. Events are converted to OTel ReadOnlySpans.
//
// Supported event phases:
//   - "X" (complete): has ts + dur
//   - "B"/"E" (begin/end): paired by name+pid+tid
//   - "i"/"I" (instant): zero-duration span
//
// Reference: https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// chromeTrace is the top-level Chrome Tracing JSON structure.
type chromeTrace struct {
	TraceEvents []chromeEvent `json:"traceEvents"`
}

// chromeEvent represents a single event in Chrome Tracing format.
type chromeEvent struct {
	Name string                 `json:"name"`
	Ph   string                 `json:"ph"`
	Ts   float64                `json:"ts"`  // microseconds
	Dur  float64                `json:"dur"` // microseconds (for "X" events)
	Pid  json.Number            `json:"pid"`
	Tid  json.Number            `json:"tid"`
	Cat  string                 `json:"cat,omitempty"`
	Args map[string]any `json:"args,omitempty"`
	ID   string                 `json:"id,omitempty"`
}

// ParseChrome reads Chrome Tracing JSON and returns ReadOnlySpans.
// Accepts either {"traceEvents": [...]} or a bare [...] array.
func ParseChrome(r io.Reader) ([]sdktrace.ReadOnlySpan, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading chrome trace: %w", err)
	}

	var events []chromeEvent

	// Try wrapped format first: {"traceEvents": [...]}
	var wrapped chromeTrace
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.TraceEvents) > 0 {
		events = wrapped.TraceEvents
	} else {
		// Try bare array: [...]
		if err := json.Unmarshal(data, &events); err != nil {
			return nil, fmt.Errorf("decode chrome trace JSON: %w", err)
		}
	}

	return chromeEventsToSpans(events)
}

func chromeEventsToSpans(events []chromeEvent) ([]sdktrace.ReadOnlySpan, error) {
	// Generate a deterministic trace ID from the events.
	traceID := syntheticTraceID(events)

	var stubs tracetest.SpanStubs

	// Collect B/E pairs keyed by name+pid+tid.
	type beginKey struct {
		Name string
		Pid  string
		Tid  string
	}
	begins := make(map[beginKey]chromeEvent)

	// Sort by timestamp to ensure B events come before their E events.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Ts < events[j].Ts
	})

	spanIndex := 0
	for _, ev := range events {
		switch ev.Ph {
		case "X": // Complete event
			stub := chromeEventToStub(ev, traceID, spanIndex)
			stubs = append(stubs, stub)
			spanIndex++

		case "B": // Begin
			key := beginKey{ev.Name, ev.Pid.String(), ev.Tid.String()}
			begins[key] = ev

		case "E": // End
			key := beginKey{ev.Name, ev.Pid.String(), ev.Tid.String()}
			if begin, ok := begins[key]; ok {
				merged := begin
				merged.Dur = ev.Ts - begin.Ts
				// Merge args from end event
				if len(ev.Args) > 0 {
					if merged.Args == nil {
						merged.Args = ev.Args
					} else {
						maps.Copy(merged.Args, ev.Args)
					}
				}
				stub := chromeEventToStub(merged, traceID, spanIndex)
				stubs = append(stubs, stub)
				spanIndex++
				delete(begins, key)
			}

		case "i", "I": // Instant event
			instant := ev
			instant.Dur = 0
			stub := chromeEventToStub(instant, traceID, spanIndex)
			stubs = append(stubs, stub)
			spanIndex++
		}
	}

	return stubs.Snapshots(), nil
}

func chromeEventToStub(ev chromeEvent, traceID trace.TraceID, index int) tracetest.SpanStub {
	spanID := syntheticSpanID(traceID, index)

	startMicros := ev.Ts
	durMicros := ev.Dur

	startTime := time.Unix(0, int64(startMicros*1000)) // micros → nanos
	endTime := time.Unix(0, int64((startMicros+durMicros)*1000))

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	var attrs []attribute.KeyValue
	if ev.Cat != "" {
		attrs = append(attrs, attribute.String("chrome.category", ev.Cat))
	}
	attrs = append(attrs, attribute.String("chrome.pid", ev.Pid.String()))
	attrs = append(attrs, attribute.String("chrome.tid", ev.Tid.String()))

	for k, v := range ev.Args {
		key := attribute.Key("chrome.args." + k)
		switch val := v.(type) {
		case string:
			attrs = append(attrs, key.String(val))
		case float64:
			attrs = append(attrs, key.Float64(val))
		case bool:
			attrs = append(attrs, key.Bool(val))
		default:
			attrs = append(attrs, key.String(fmt.Sprintf("%v", val)))
		}
	}

	return tracetest.SpanStub{
		Name:        ev.Name,
		SpanContext: sc,
		StartTime:   startTime,
		EndTime:     endTime,
		Attributes:  attrs,
	}
}

// syntheticTraceID generates a deterministic trace ID by hashing event data.
func syntheticTraceID(events []chromeEvent) trace.TraceID {
	h := sha256.New()
	for _, ev := range events {
		fmt.Fprintf(h, "%s:%s:%.0f", ev.Name, ev.Ph, ev.Ts)
	}
	var tid trace.TraceID
	copy(tid[:], h.Sum(nil)[:16])
	return tid
}

// syntheticSpanID generates a deterministic span ID from trace ID and index.
func syntheticSpanID(traceID trace.TraceID, index int) trace.SpanID {
	h := sha256.New()
	h.Write(traceID[:])
	binary.Write(h, binary.BigEndian, int64(index))
	var sid trace.SpanID
	copy(sid[:], h.Sum(nil)[:8])
	return sid
}
