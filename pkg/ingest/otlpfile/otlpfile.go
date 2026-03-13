// Package otlpfile parses OTel span JSON files as produced by the
// stdouttrace exporter (--otel flag) into ReadOnlySpan slices.
//
// The format is newline-delimited JSON where each line is a span object:
//
//	{"Name":"...","SpanContext":{...},"Parent":{...},"StartTime":"...","EndTime":"...","Attributes":[...], ...}
package otlpfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// stdoutSpan is the JSON structure emitted by stdouttrace exporter.
type stdoutSpan struct {
	Name        string          `json:"Name"`
	SpanContext spanContextJSON `json:"SpanContext"`
	Parent      spanContextJSON `json:"Parent"`
	SpanKind    int             `json:"SpanKind"`
	StartTime   time.Time       `json:"StartTime"`
	EndTime     time.Time       `json:"EndTime"`
	Attributes  []attrJSON      `json:"Attributes"`
	Events      []eventJSON     `json:"Events"`
	Links       []linkJSON      `json:"Links"`
	Status      statusJSON      `json:"Status"`
}

type spanContextJSON struct {
	TraceID    string `json:"TraceID"`
	SpanID     string `json:"SpanID"`
	TraceFlags string `json:"TraceFlags"`
}

type attrJSON struct {
	Key   string    `json:"Key"`
	Value valueJSON `json:"Value"`
}

type valueJSON struct {
	Type  string      `json:"Type"`
	Value interface{} `json:"Value"`
}

type eventJSON struct {
	Name       string     `json:"Name"`
	Attributes []attrJSON `json:"Attributes"`
	Time       time.Time  `json:"Time"`
}

type linkJSON struct {
	SpanContext spanContextJSON `json:"SpanContext"`
	Attributes  []attrJSON     `json:"Attributes"`
}

type statusJSON struct {
	Code        string `json:"Code"`
	Description string `json:"Description"`
}

// ParseFile reads an OTel JSON file and returns ReadOnlySpans.
// Auto-detects format: OTLP protobuf-JSON (ExportTraceServiceRequest with
// "resourceSpans") or stdouttrace (newline-delimited/array of span objects).
func ParseFile(path string) ([]sdktrace.ReadOnlySpan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads OTel span JSON from a reader and returns ReadOnlySpans.
// Auto-detects format: OTLP protobuf-JSON ("resourceSpans"), Chrome Tracing
// ("traceEvents"/"ph"), flat JSON ("ParentSpanID" with map-style attributes),
// or stdouttrace (newline-delimited/array).
func Parse(r io.Reader) ([]sdktrace.ReadOnlySpan, error) {
	// Read all content so we can inspect it for format detection.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading trace data: %w", err)
	}

	// Detect OTLP protobuf-JSON format by looking for "resourceSpans" key.
	if bytes.Contains(data, []byte(`"resourceSpans"`)) {
		return ParseProto(bytes.NewReader(data))
	}

	// Detect Chrome Tracing format by looking for "traceEvents" key or
	// "ph" field (event phase indicator unique to Chrome Tracing).
	if bytes.Contains(data, []byte(`"traceEvents"`)) || looksLikeChromeTrace(data) {
		return ParseChrome(bytes.NewReader(data))
	}

	// Detect flat JSON format: has "ParentSpanID" (stdouttrace uses "Parent").
	if bytes.Contains(data, []byte(`"ParentSpanID"`)) {
		return parseFlatJSON(data)
	}

	return parseStdout(bytes.NewReader(data))
}

// parseStdout reads OTel stdouttrace JSON (newline-delimited or array).
func parseStdout(r io.Reader) ([]sdktrace.ReadOnlySpan, error) {
	var stubs tracetest.SpanStubs

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		trimmed := trimSpace(line)
		if len(trimmed) == 0 || trimmed[0] == '[' || trimmed[0] == ']' {
			continue
		}

		// Remove trailing comma for array format
		if trimmed[len(trimmed)-1] == ',' {
			trimmed = trimmed[:len(trimmed)-1]
		}

		var raw stdoutSpan
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			continue
		}

		stub, err := convertToStub(raw)
		if err != nil {
			continue
		}
		stubs = append(stubs, stub)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading trace file: %w", err)
	}

	return stubs.Snapshots(), nil
}

func convertToStub(raw stdoutSpan) (tracetest.SpanStub, error) {
	sc, err := parseSpanContext(raw.SpanContext)
	if err != nil {
		return tracetest.SpanStub{}, err
	}

	parent, _ := parseSpanContext(raw.Parent)

	attrs := convertAttrs(raw.Attributes)
	status := StatusFromCode(raw.Status.Code, raw.Status.Description)

	var events []sdktrace.Event
	for _, e := range raw.Events {
		events = append(events, sdktrace.Event{
			Name:       e.Name,
			Attributes: convertAttrs(e.Attributes),
			Time:       e.Time,
		})
	}

	var links []sdktrace.Link
	for _, l := range raw.Links {
		lsc, _ := parseSpanContext(l.SpanContext)
		links = append(links, sdktrace.Link{
			SpanContext: lsc,
			Attributes:  convertAttrs(l.Attributes),
		})
	}

	return tracetest.SpanStub{
		Name:        raw.Name,
		SpanContext: sc,
		Parent:      parent,
		SpanKind:    trace.SpanKind(raw.SpanKind),
		StartTime:   raw.StartTime,
		EndTime:     raw.EndTime,
		Attributes:  attrs,
		Events:      events,
		Links:       links,
		Status:      status,
	}, nil
}

func parseSpanContext(sc spanContextJSON) (trace.SpanContext, error) {
	if sc.TraceID == "" && sc.SpanID == "" {
		return trace.SpanContext{}, nil
	}

	traceID, err := trace.TraceIDFromHex(sc.TraceID)
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("invalid trace ID %q: %w", sc.TraceID, err)
	}

	spanID, err := trace.SpanIDFromHex(sc.SpanID)
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("invalid span ID %q: %w", sc.SpanID, err)
	}

	cfg := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}

	return trace.NewSpanContext(cfg), nil
}

func convertAttrs(raw []attrJSON) []attribute.KeyValue {
	var result []attribute.KeyValue
	for _, a := range raw {
		result = append(result, convertAttr(a))
	}
	return result
}

func convertAttr(a attrJSON) attribute.KeyValue {
	key := attribute.Key(a.Key)
	switch a.Value.Type {
	case "STRING":
		if s, ok := a.Value.Value.(string); ok {
			return key.String(s)
		}
	case "INT64":
		switch v := a.Value.Value.(type) {
		case float64:
			return key.Int64(int64(v))
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return key.Int64(i)
			}
		}
	case "FLOAT64":
		if f, ok := a.Value.Value.(float64); ok {
			return key.Float64(f)
		}
	case "BOOL":
		if b, ok := a.Value.Value.(bool); ok {
			return key.Bool(b)
		}
	}
	return key.String(fmt.Sprintf("%v", a.Value.Value))
}

// looksLikeChromeTrace checks for bare-array Chrome Tracing format
// by looking for the "ph" key (event phase) which is unique to Chrome Tracing.
func looksLikeChromeTrace(data []byte) bool {
	// Check for "ph": pattern — the "ph" JSON key followed by a colon.
	for i := 0; i < len(data)-4; i++ {
		if data[i] == '"' && data[i+1] == 'p' && data[i+2] == 'h' && data[i+3] == '"' {
			// Look for colon after optional whitespace
			for j := i + 4; j < len(data); j++ {
				if data[j] == ':' {
					return true
				}
				if data[j] != ' ' && data[j] != '\t' {
					break
				}
			}
		}
	}
	return false
}

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}

// flatSpan is the JSON structure used by some Go OTel exporters that serialize
// attributes as a plain map and use "ParentSpanID" instead of a "Parent" object.
type flatSpan struct {
	Name         string                 `json:"Name"`
	SpanContext  spanContextJSON        `json:"SpanContext"`
	ParentSpanID string                 `json:"ParentSpanID"`
	SpanKind     int                    `json:"SpanKind"`
	StartTime    time.Time              `json:"StartTime"`
	EndTime      time.Time              `json:"EndTime"`
	Attributes   map[string]interface{} `json:"Attributes"`
	Events       []flatEventJSON        `json:"Events"`
	Status       statusJSON             `json:"Status"`
	Resource     map[string]interface{} `json:"Resource"`
}

type flatEventJSON struct {
	Name       string                 `json:"Name"`
	Attributes map[string]interface{} `json:"Attributes"`
	Time       time.Time              `json:"Time"`
}

// parseFlatJSON parses spans with flat attribute maps and "ParentSpanID".
// Supports a single object, newline-delimited objects, or a JSON array.
func parseFlatJSON(data []byte) ([]sdktrace.ReadOnlySpan, error) {
	var stubs tracetest.SpanStubs

	// Try single object first.
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var raw flatSpan
		if err := json.Unmarshal(trimmed, &raw); err == nil {
			if stub, err := convertFlatToStub(raw); err == nil {
				stubs = append(stubs, stub)
				return stubs.Snapshots(), nil
			}
		}
	}

	// Try JSON array.
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var raws []flatSpan
		if err := json.Unmarshal(trimmed, &raws); err == nil {
			for _, raw := range raws {
				if stub, err := convertFlatToStub(raw); err == nil {
					stubs = append(stubs, stub)
				}
			}
			return stubs.Snapshots(), nil
		}
	}

	// Fall back to newline-delimited.
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := trimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if line[len(line)-1] == ',' {
			line = line[:len(line)-1]
		}
		var raw flatSpan
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if stub, err := convertFlatToStub(raw); err == nil {
			stubs = append(stubs, stub)
		}
	}
	return stubs.Snapshots(), nil
}

func convertFlatToStub(raw flatSpan) (tracetest.SpanStub, error) {
	sc, err := parseSpanContext(raw.SpanContext)
	if err != nil {
		return tracetest.SpanStub{}, err
	}

	// Build parent span context from ParentSpanID + same TraceID.
	var parent trace.SpanContext
	if raw.ParentSpanID != "" && raw.ParentSpanID != "0000000000000000" {
		parentSpanID, err := trace.SpanIDFromHex(raw.ParentSpanID)
		if err == nil {
			parent = trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    sc.TraceID(),
				SpanID:     parentSpanID,
				TraceFlags: trace.FlagsSampled,
			})
		}
	}

	attrs := convertFlatAttrs(raw.Attributes)

	// Add resource attributes with "resource." prefix.
	for k, v := range raw.Resource {
		attrs = append(attrs, flatAttr("resource."+k, v))
	}

	status := StatusFromCode(raw.Status.Code, raw.Status.Description)

	var events []sdktrace.Event
	for _, e := range raw.Events {
		events = append(events, sdktrace.Event{
			Name:       e.Name,
			Attributes: convertFlatAttrs(e.Attributes),
			Time:       e.Time,
		})
	}

	return tracetest.SpanStub{
		Name:        raw.Name,
		SpanContext: sc,
		Parent:      parent,
		SpanKind:    trace.SpanKind(raw.SpanKind),
		StartTime:   raw.StartTime,
		EndTime:     raw.EndTime,
		Attributes:  attrs,
		Events:      events,
		Status:      status,
	}, nil
}

func convertFlatAttrs(m map[string]interface{}) []attribute.KeyValue {
	var result []attribute.KeyValue
	for k, v := range m {
		result = append(result, flatAttr(k, v))
	}
	return result
}

// flatAttr converts a key and an untyped JSON value to an attribute.KeyValue,
// inferring the type from the Go type that encoding/json produces.
func flatAttr(k string, v interface{}) attribute.KeyValue {
	key := attribute.Key(k)
	switch val := v.(type) {
	case string:
		return key.String(val)
	case float64:
		// JSON numbers are float64; use int if it's a whole number.
		if val == float64(int64(val)) {
			return key.Int64(int64(val))
		}
		return key.Float64(val)
	case bool:
		return key.Bool(val)
	default:
		return key.String(fmt.Sprintf("%v", v))
	}
}
