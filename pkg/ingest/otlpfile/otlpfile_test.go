package otlpfile

import (
	"strings"
	"testing"
	"time"
)

func TestParseNewlineDelimited(t *testing.T) {
	input := `{"Name":"my-workflow","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:05:00Z","Attributes":[{"Key":"type","Value":{"Type":"STRING","Value":"workflow"}},{"Key":"github.conclusion","Value":{"Type":"STRING","Value":"success"}}],"Events":null,"Links":null,"Status":{"Code":"OK","Description":""}}
{"Name":"build","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"00f067aa0ba902b7","TraceFlags":"01"},"Parent":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331"},"SpanKind":1,"StartTime":"2024-01-15T10:00:30Z","EndTime":"2024-01-15T10:04:00Z","Attributes":[{"Key":"type","Value":{"Type":"STRING","Value":"job"}}],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Check first span
	if spans[0].Name() != "my-workflow" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "my-workflow")
	}
	if spans[0].SpanContext().TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("span[0] traceID = %q", spans[0].SpanContext().TraceID().String())
	}
	if !spans[0].Parent().SpanID().IsValid() {
		// Parent has empty IDs, so it should be invalid
	} else {
		t.Errorf("span[0] should have invalid parent span ID")
	}

	// Check second span has parent
	if spans[1].Name() != "build" {
		t.Errorf("span[1] name = %q, want %q", spans[1].Name(), "build")
	}
	if spans[1].Parent().SpanID().String() != "b7ad6b7169203331" {
		t.Errorf("span[1] parent spanID = %q, want %q", spans[1].Parent().SpanID().String(), "b7ad6b7169203331")
	}

	// Check attributes
	attrs := spans[0].Attributes()
	found := false
	for _, a := range attrs {
		if string(a.Key) == "type" && a.Value.AsString() == "workflow" {
			found = true
		}
	}
	if !found {
		t.Errorf("span[0] missing type=workflow attribute")
	}

	// Check timing
	expectedStart := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if !spans[0].StartTime().Equal(expectedStart) {
		t.Errorf("span[0] start = %v, want %v", spans[0].StartTime(), expectedStart)
	}
}

func TestParseArrayFormat(t *testing.T) {
	input := `[
  {"Name":"step1","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:01:00Z","Attributes":[],"Events":null,"Links":null,"Status":{"Code":"","Description":""}},
  {"Name":"step2","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"00f067aa0ba902b7","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:01:00Z","EndTime":"2024-01-15T10:02:00Z","Attributes":[],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}
]`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Name() != "step1" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "step1")
	}
	if spans[1].Name() != "step2" {
		t.Errorf("span[1] name = %q, want %q", spans[1].Name(), "step2")
	}
}

func TestParseEmptyInput(t *testing.T) {
	spans, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(spans))
	}
}

func TestParseSkipsMalformedLines(t *testing.T) {
	input := `not valid json
{"Name":"ok","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:01:00Z","Attributes":[],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}
also not valid`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "ok" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "ok")
	}
}

func TestParseWithEvents(t *testing.T) {
	input := `{"Name":"with-events","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:01:00Z","Attributes":[],"Events":[{"Name":"cache-hit","Attributes":[{"Key":"cache.type","Value":{"Type":"STRING","Value":"npm"}}],"Time":"2024-01-15T10:00:30Z"}],"Links":null,"Status":{"Code":"ERROR","Description":"build failed"}}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	events := spans[0].Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "cache-hit" {
		t.Errorf("event name = %q, want %q", events[0].Name, "cache-hit")
	}
}

func TestParseInt64Attribute(t *testing.T) {
	input := `{"Name":"int-attr","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:01:00Z","Attributes":[{"Key":"http.status_code","Value":{"Type":"INT64","Value":200}}],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	attrs := spans[0].Attributes()
	found := false
	for _, a := range attrs {
		if string(a.Key) == "http.status_code" && a.Value.AsInt64() == 200 {
			found = true
		}
	}
	if !found {
		t.Errorf("missing http.status_code=200 attribute, got %v", attrs)
	}
}

func TestParseProtoBasic(t *testing.T) {
	input := `{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"traceId": "0af7651916cd43dd8448eb211c80319c",
					"spanId": "b7ad6b7169203331",
					"parentSpanId": "",
					"name": "HTTP GET",
					"kind": 2,
					"startTimeUnixNano": "1705312800000000000",
					"endTimeUnixNano":   "1705312801000000000",
					"attributes": [
						{"key": "http.method", "value": {"stringValue": "GET"}},
						{"key": "http.status_code", "value": {"intValue": "200"}}
					],
					"status": {"code": 1}
				}, {
					"traceId": "0af7651916cd43dd8448eb211c80319c",
					"spanId": "00f067aa0ba902b7",
					"parentSpanId": "b7ad6b7169203331",
					"name": "db.query",
					"kind": 3,
					"startTimeUnixNano": "1705312800100000000",
					"endTimeUnixNano":   "1705312800500000000",
					"attributes": [
						{"key": "db.system", "value": {"stringValue": "postgresql"}}
					],
					"status": {"code": 0}
				}]
			}]
		}]
	}`

	spans, err := ParseProto(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// First span
	if spans[0].Name() != "HTTP GET" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "HTTP GET")
	}
	if spans[0].SpanContext().TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("span[0] traceID = %q", spans[0].SpanContext().TraceID().String())
	}
	if spans[0].Parent().SpanID().IsValid() {
		t.Errorf("span[0] should have no parent")
	}

	// Second span has parent
	if spans[1].Parent().SpanID().String() != "b7ad6b7169203331" {
		t.Errorf("span[1] parent spanID = %q, want %q", spans[1].Parent().SpanID().String(), "b7ad6b7169203331")
	}

	// Check attributes on first span
	attrs := spans[0].Attributes()
	var foundMethod, foundStatus bool
	for _, a := range attrs {
		if string(a.Key) == "http.method" && a.Value.AsString() == "GET" {
			foundMethod = true
		}
		if string(a.Key) == "http.status_code" && a.Value.AsInt64() == 200 {
			foundStatus = true
		}
	}
	if !foundMethod {
		t.Errorf("missing http.method=GET attribute")
	}
	if !foundStatus {
		t.Errorf("missing http.status_code=200 attribute")
	}

	// Check timing
	expectedStart := time.Unix(0, 1705312800000000000)
	if !spans[0].StartTime().Equal(expectedStart) {
		t.Errorf("span[0] start = %v, want %v", spans[0].StartTime(), expectedStart)
	}
}

func TestParseProtoNumericNanos(t *testing.T) {
	// Some backends emit timestamps as numbers instead of strings
	input := `{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"traceId": "0af7651916cd43dd8448eb211c80319c",
					"spanId": "b7ad6b7169203331",
					"name": "numeric-ts",
					"kind": 1,
					"startTimeUnixNano": 1705312800000000000,
					"endTimeUnixNano":   1705312801000000000,
					"status": {"code": 2, "message": "something broke"}
				}]
			}]
		}]
	}`

	spans, err := ParseProto(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "numeric-ts" {
		t.Errorf("name = %q", spans[0].Name())
	}
	// Status should be Error
	if spans[0].Status().Code.String() != "Error" {
		t.Errorf("status = %v, want Error", spans[0].Status().Code)
	}
}

func TestParseProtoWithEvents(t *testing.T) {
	input := `{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"traceId": "0af7651916cd43dd8448eb211c80319c",
					"spanId": "b7ad6b7169203331",
					"name": "with-events",
					"kind": 1,
					"startTimeUnixNano": "1705312800000000000",
					"endTimeUnixNano":   "1705312801000000000",
					"events": [{
						"name": "exception",
						"timeUnixNano": "1705312800500000000",
						"attributes": [
							{"key": "exception.message", "value": {"stringValue": "null pointer"}}
						]
					}],
					"status": {"code": 0}
				}]
			}]
		}]
	}`

	spans, err := ParseProto(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	events := spans[0].Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "exception" {
		t.Errorf("event name = %q, want %q", events[0].Name, "exception")
	}
}

func TestParseAutoDetectsProtoFormat(t *testing.T) {
	// Parse() should auto-detect protobuf-JSON format
	input := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"0af7651916cd43dd8448eb211c80319c","spanId":"b7ad6b7169203331","name":"auto-detected","kind":1,"startTimeUnixNano":"1705312800000000000","endTimeUnixNano":"1705312801000000000","status":{"code":0}}]}]}]}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "auto-detected" {
		t.Errorf("name = %q, want %q", spans[0].Name(), "auto-detected")
	}
}

func TestParseProtoAttributeTypes(t *testing.T) {
	input := `{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"traceId": "0af7651916cd43dd8448eb211c80319c",
					"spanId": "b7ad6b7169203331",
					"name": "attr-types",
					"kind": 1,
					"startTimeUnixNano": "1705312800000000000",
					"endTimeUnixNano":   "1705312801000000000",
					"attributes": [
						{"key": "str", "value": {"stringValue": "hello"}},
						{"key": "num", "value": {"intValue": "42"}},
						{"key": "dbl", "value": {"doubleValue": 3.14}},
						{"key": "flag", "value": {"boolValue": true}}
					],
					"status": {"code": 0}
				}]
			}]
		}]
	}`

	spans, err := ParseProto(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	attrs := spans[0].Attributes()
	if len(attrs) != 4 {
		t.Fatalf("expected 4 attrs, got %d", len(attrs))
	}

	checks := map[string]bool{"str": false, "num": false, "dbl": false, "flag": false}
	for _, a := range attrs {
		switch string(a.Key) {
		case "str":
			if a.Value.AsString() == "hello" {
				checks["str"] = true
			}
		case "num":
			if a.Value.AsInt64() == 42 {
				checks["num"] = true
			}
		case "dbl":
			if a.Value.AsFloat64() == 3.14 {
				checks["dbl"] = true
			}
		case "flag":
			if a.Value.AsBool() == true {
				checks["flag"] = true
			}
		}
	}
	for k, v := range checks {
		if !v {
			t.Errorf("attribute %q not found or wrong value", k)
		}
	}
}

func TestParseChromeTraceWrapped(t *testing.T) {
	input := `{
		"traceEvents": [
			{"name":"CompileC","ph":"X","ts":1000000,"dur":500000,"pid":1,"tid":1,"cat":"cc","args":{"file":"main.c"}},
			{"name":"Link","ph":"X","ts":1500000,"dur":200000,"pid":1,"tid":2,"cat":"link"}
		]
	}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	if spans[0].Name() != "CompileC" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "CompileC")
	}
	if spans[1].Name() != "Link" {
		t.Errorf("span[1] name = %q, want %q", spans[1].Name(), "Link")
	}

	// Check timing: 1000000 microseconds = 1 second
	expectedStart := time.Unix(1, 0)
	if !spans[0].StartTime().Equal(expectedStart) {
		t.Errorf("span[0] start = %v, want %v", spans[0].StartTime(), expectedStart)
	}
	expectedEnd := time.Unix(1, 500000000) // 1.5 seconds
	if !spans[0].EndTime().Equal(expectedEnd) {
		t.Errorf("span[0] end = %v, want %v", spans[0].EndTime(), expectedEnd)
	}

	// Check attributes
	attrs := spans[0].Attributes()
	var foundCat, foundArg bool
	for _, a := range attrs {
		if string(a.Key) == "chrome.category" && a.Value.AsString() == "cc" {
			foundCat = true
		}
		if string(a.Key) == "chrome.args.file" && a.Value.AsString() == "main.c" {
			foundArg = true
		}
	}
	if !foundCat {
		t.Errorf("missing chrome.category=cc attribute")
	}
	if !foundArg {
		t.Errorf("missing chrome.args.file=main.c attribute")
	}
}

func TestParseChromeTraceBareArray(t *testing.T) {
	input := `[
		{"name":"Task","ph":"X","ts":0,"dur":100000,"pid":1,"tid":1},
		{"name":"Subtask","ph":"X","ts":10000,"dur":50000,"pid":1,"tid":1}
	]`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Name() != "Task" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "Task")
	}
}

func TestParseChromeTraceBeginEnd(t *testing.T) {
	input := `{"traceEvents": [
		{"name":"LongOp","ph":"B","ts":1000000,"pid":1,"tid":1},
		{"name":"LongOp","ph":"E","ts":2000000,"pid":1,"tid":1,"args":{"result":"ok"}}
	]}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "LongOp" {
		t.Errorf("name = %q, want %q", spans[0].Name(), "LongOp")
	}

	// Duration should be 1 second
	dur := spans[0].EndTime().Sub(spans[0].StartTime())
	if dur != time.Second {
		t.Errorf("duration = %v, want 1s", dur)
	}

	// Check merged args from E event
	var foundResult bool
	for _, a := range spans[0].Attributes() {
		if string(a.Key) == "chrome.args.result" && a.Value.AsString() == "ok" {
			foundResult = true
		}
	}
	if !foundResult {
		t.Errorf("missing chrome.args.result=ok from end event")
	}
}

func TestParseChromeTraceInstant(t *testing.T) {
	input := `{"traceEvents": [
		{"name":"Marker","ph":"i","ts":5000000,"pid":1,"tid":1,"s":"g"}
	]}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "Marker" {
		t.Errorf("name = %q, want %q", spans[0].Name(), "Marker")
	}
	// Instant events should have zero duration
	if !spans[0].StartTime().Equal(spans[0].EndTime()) {
		t.Errorf("instant event should have start == end")
	}
}

func TestParseChromeTraceSkipsMetadata(t *testing.T) {
	input := `{"traceEvents": [
		{"name":"process_name","ph":"M","pid":1,"args":{"name":"Browser"}},
		{"name":"RealWork","ph":"X","ts":1000,"dur":500,"pid":1,"tid":1}
	]}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span (metadata skipped), got %d", len(spans))
	}
	if spans[0].Name() != "RealWork" {
		t.Errorf("name = %q, want %q", spans[0].Name(), "RealWork")
	}
}

func TestParseFlatJSONSingleObject(t *testing.T) {
	input := `{
		"Name": "GET /user",
		"SpanContext": {
			"TraceID": "384719368474cf130bdd39cffbe0781f",
			"SpanID": "eb123f7615b18f36",
			"TraceFlags": "01"
		},
		"ParentSpanID": "0000000000000000",
		"SpanKind": 2,
		"StartTime": "2026-03-12T10:00:00.123456Z",
		"EndTime": "2026-03-12T10:00:00.125678Z",
		"Attributes": {
			"http.method": "GET",
			"http.route": "/user",
			"http.status_code": 200,
			"net.peer.ip": "192.168.1.10"
		},
		"Events": [
			{
				"Name": "database_query",
				"Attributes": {
					"db.statement": "SELECT * FROM users WHERE id = 1"
				},
				"Time": "2026-03-12T10:00:00.124000Z"
			}
		],
		"Resource": {
			"service.name": "user-service",
			"service.version": "1.2.0"
		},
		"Status": {
			"Code": "Ok"
		}
	}`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name() != "GET /user" {
		t.Errorf("name = %q, want %q", s.Name(), "GET /user")
	}
	if s.SpanContext().TraceID().String() != "384719368474cf130bdd39cffbe0781f" {
		t.Errorf("traceID = %q", s.SpanContext().TraceID().String())
	}
	// ParentSpanID all zeros → no parent
	if s.Parent().SpanID().IsValid() {
		t.Errorf("expected invalid parent for all-zero ParentSpanID")
	}

	// Check attributes
	attrMap := make(map[string]string)
	for _, a := range s.Attributes() {
		attrMap[string(a.Key)] = a.Value.Emit()
	}
	if attrMap["http.method"] != "GET" {
		t.Errorf("http.method = %q, want %q", attrMap["http.method"], "GET")
	}
	if attrMap["http.status_code"] != "200" {
		t.Errorf("http.status_code = %q, want %q", attrMap["http.status_code"], "200")
	}
	if attrMap["resource.service.name"] != "user-service" {
		t.Errorf("resource.service.name = %q, want %q", attrMap["resource.service.name"], "user-service")
	}

	// Check events
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "database_query" {
		t.Errorf("event name = %q", events[0].Name)
	}
	var foundStmt bool
	for _, a := range events[0].Attributes {
		if string(a.Key) == "db.statement" {
			foundStmt = true
		}
	}
	if !foundStmt {
		t.Errorf("missing db.statement event attribute")
	}

	// Check status
	if s.Status().Code.String() != "Ok" {
		t.Errorf("status = %v, want Ok", s.Status().Code)
	}
}

func TestParseFlatJSONWithParent(t *testing.T) {
	input := `[
		{
			"Name": "root",
			"SpanContext": {"TraceID": "384719368474cf130bdd39cffbe0781f", "SpanID": "eb123f7615b18f36", "TraceFlags": "01"},
			"ParentSpanID": "0000000000000000",
			"SpanKind": 1,
			"StartTime": "2026-03-12T10:00:00Z",
			"EndTime": "2026-03-12T10:00:01Z",
			"Attributes": {},
			"Status": {"Code": ""}
		},
		{
			"Name": "child",
			"SpanContext": {"TraceID": "384719368474cf130bdd39cffbe0781f", "SpanID": "aa00bb11cc22dd33", "TraceFlags": "01"},
			"ParentSpanID": "eb123f7615b18f36",
			"SpanKind": 1,
			"StartTime": "2026-03-12T10:00:00.1Z",
			"EndTime": "2026-03-12T10:00:00.5Z",
			"Attributes": {"custom.tag": "hello"},
			"Status": {"Code": ""}
		}
	]`

	spans, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Child should have parent pointing to root
	child := spans[1]
	if child.Name() != "child" {
		t.Errorf("span[1] name = %q, want %q", child.Name(), "child")
	}
	if child.Parent().SpanID().String() != "eb123f7615b18f36" {
		t.Errorf("child parent spanID = %q, want %q", child.Parent().SpanID().String(), "eb123f7615b18f36")
	}
}

func TestStatusFromCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"OK", "Ok"},
		{"STATUS_CODE_OK", "Ok"},
		{"ERROR", "Error"},
		{"STATUS_CODE_ERROR", "Error"},
		{"UNSET", "Unset"},
		{"", "Unset"},
	}
	for _, tt := range tests {
		st := StatusFromCode(tt.code, "desc")
		if st.Code.String() != tt.want {
			t.Errorf("StatusFromCode(%q) = %v, want %v", tt.code, st.Code, tt.want)
		}
	}
}
