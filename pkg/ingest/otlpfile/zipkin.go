package otlpfile

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// zipkinSpan is the JSON structure for a Zipkin v2 span.
type zipkinSpan struct {
	TraceID       string            `json:"traceId"`
	ID            string            `json:"id"`
	ParentID      string            `json:"parentId,omitempty"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind,omitempty"`
	Timestamp     int64             `json:"timestamp"`
	Duration      int64             `json:"duration"`
	LocalEndpoint *zipkinEndpoint   `json:"localEndpoint,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// zipkinEndpoint represents a Zipkin endpoint.
type zipkinEndpoint struct {
	ServiceName string `json:"serviceName"`
	IPv4        string `json:"ipv4,omitempty"`
	IPv6        string `json:"ipv6,omitempty"`
	Port        int    `json:"port,omitempty"`
}

// ParseZipkin reads Zipkin v2 JSON (array of spans) and returns ReadOnlySpans.
func ParseZipkin(r io.Reader) ([]sdktrace.ReadOnlySpan, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading zipkin data: %w", err)
	}

	var raw []zipkinSpan
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing zipkin JSON: %w", err)
	}

	var stubs tracetest.SpanStubs
	for _, zs := range raw {
		stub, err := convertZipkinToStub(zs)
		if err != nil {
			continue
		}
		stubs = append(stubs, stub)
	}

	return stubs.Snapshots(), nil
}

func convertZipkinToStub(zs zipkinSpan) (tracetest.SpanStub, error) {
	// Handle 16-char (64-bit) trace IDs by zero-padding to 32 chars.
	traceIDHex := zs.TraceID
	if len(traceIDHex) == 16 {
		traceIDHex = strings.Repeat("0", 16) + traceIDHex
	}

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		return tracetest.SpanStub{}, fmt.Errorf("invalid trace ID %q: %w", zs.TraceID, err)
	}

	spanID, err := trace.SpanIDFromHex(zs.ID)
	if err != nil {
		return tracetest.SpanStub{}, fmt.Errorf("invalid span ID %q: %w", zs.ID, err)
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	// Build parent span context.
	var parent trace.SpanContext
	if zs.ParentID != "" {
		parentSpanID, err := trace.SpanIDFromHex(zs.ParentID)
		if err == nil {
			parent = trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				SpanID:     parentSpanID,
				TraceFlags: trace.FlagsSampled,
			})
		}
	}

	// Convert timestamps: Zipkin uses microseconds since epoch.
	startTime := time.Unix(0, zs.Timestamp*1000) // microseconds → nanoseconds
	endTime := time.Unix(0, (zs.Timestamp+zs.Duration)*1000)

	// Convert tags to attributes.
	var attrs []attribute.KeyValue
	var res *resource.Resource
	if zs.LocalEndpoint != nil && zs.LocalEndpoint.ServiceName != "" {
		attrs = append(attrs, attribute.String("service.name", zs.LocalEndpoint.ServiceName))
		res = resource.NewSchemaless(attribute.String("service.name", zs.LocalEndpoint.ServiceName))
	}
	for k, v := range zs.Tags {
		attrs = append(attrs, attribute.String(k, v))
	}

	return tracetest.SpanStub{
		Name:        zs.Name,
		SpanContext: sc,
		Parent:      parent,
		SpanKind:    zipkinKind(zs.Kind),
		StartTime:   startTime,
		EndTime:     endTime,
		Attributes:  attrs,
		Resource:    res,
	}, nil
}

// zipkinKind maps Zipkin kind strings to OTel SpanKind values.
func zipkinKind(kind string) trace.SpanKind {
	switch strings.ToUpper(kind) {
	case "CLIENT":
		return trace.SpanKindClient
	case "SERVER":
		return trace.SpanKindServer
	case "PRODUCER":
		return trace.SpanKindProducer
	case "CONSUMER":
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindUnspecified
	}
}
