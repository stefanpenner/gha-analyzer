package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// makeSpans creates real ReadOnlySpan values using an in-memory tracer provider.
func makeSpans(t *testing.T, names ...string) []sdktrace.ReadOnlySpan {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	ctx := context.Background()
	for _, name := range names {
		_, span := tracer.Start(ctx, name)
		span.End()
	}
	tp.ForceFlush(ctx)

	stubs := exporter.GetSpans()
	spans := make([]sdktrace.ReadOnlySpan, len(stubs))
	for i := range stubs {
		spans[i] = stubs[i].Snapshot()
	}
	return spans
}

// makeSpanWithAttrs creates a span with the given attributes.
func makeSpanWithAttrs(t *testing.T, name string, attrs ...attribute.KeyValue) []sdktrace.ReadOnlySpan {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	span.End()
	tp.ForceFlush(ctx)

	stubs := exporter.GetSpans()
	spans := make([]sdktrace.ReadOnlySpan, len(stubs))
	for i := range stubs {
		spans[i] = stubs[i].Snapshot()
	}
	return spans
}

func TestStdoutExporter(t *testing.T) {
	tests := []struct {
		name      string
		spans     func(t *testing.T) []sdktrace.ReadOnlySpan
		checkFunc func(t *testing.T, output []byte)
	}{
		{
			name: "single span",
			spans: func(t *testing.T) []sdktrace.ReadOnlySpan {
				return makeSpans(t, "test-span")
			},
			checkFunc: func(t *testing.T, output []byte) {
				if len(output) == 0 {
					t.Fatal("expected output, got empty")
				}
				// stdouttrace writes JSON; verify it's valid
				lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
				for _, line := range lines {
					line = bytes.TrimSpace(line)
					if len(line) == 0 {
						continue
					}
					var obj map[string]interface{}
					if err := json.Unmarshal(line, &obj); err != nil {
						t.Errorf("invalid JSON line: %s\nerror: %v", line, err)
					}
				}
			},
		},
		{
			name: "multiple spans",
			spans: func(t *testing.T) []sdktrace.ReadOnlySpan {
				return makeSpans(t, "span-a", "span-b", "span-c")
			},
			checkFunc: func(t *testing.T, output []byte) {
				if len(output) == 0 {
					t.Fatal("expected output for multiple spans")
				}
				// Should contain all span names
				s := string(output)
				for _, name := range []string{"span-a", "span-b", "span-c"} {
					if !bytes.Contains(output, []byte(name)) {
						t.Errorf("output missing span name %q in: %s", name, s)
					}
				}
			},
		},
		{
			name: "empty spans",
			spans: func(t *testing.T) []sdktrace.ReadOnlySpan {
				return nil
			},
			checkFunc: func(t *testing.T, output []byte) {
				if len(bytes.TrimSpace(output)) != 0 {
					t.Errorf("expected no output for empty spans, got: %s", output)
				}
			},
		},
		{
			name: "span attributes preserved",
			spans: func(t *testing.T) []sdktrace.ReadOnlySpan {
				return makeSpanWithAttrs(t, "attr-span",
					attribute.String("test.key", "test-value"),
					attribute.Int("test.count", 42),
				)
			},
			checkFunc: func(t *testing.T, output []byte) {
				s := string(output)
				if !bytes.Contains(output, []byte("test.key")) {
					t.Errorf("output missing attribute key 'test.key' in: %s", s)
				}
				if !bytes.Contains(output, []byte("test-value")) {
					t.Errorf("output missing attribute value 'test-value' in: %s", s)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			exp, err := NewStdoutExporter(&buf)
			if err != nil {
				t.Fatalf("NewStdoutExporter failed: %v", err)
			}

			ctx := context.Background()
			spans := tt.spans(t)
			if len(spans) > 0 {
				if err := exp.Export(ctx, spans); err != nil {
					t.Fatalf("Export failed: %v", err)
				}
			}

			tt.checkFunc(t, buf.Bytes())
		})
	}
}

func TestStdoutExporterFinish(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewStdoutExporter(&buf)
	if err != nil {
		t.Fatalf("NewStdoutExporter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exp.Finish(ctx); err != nil {
		t.Errorf("Finish returned error: %v", err)
	}
}

func TestGRPCExporter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// gRPC connects lazily, so constructor succeeds without a running server
	exp, err := NewGRPCExporter(ctx, "localhost:4317")
	if err != nil {
		t.Fatalf("NewGRPCExporter failed: %v", err)
	}
	if exp == nil {
		t.Fatal("NewGRPCExporter returned nil exporter")
	}

	// Finish (shutdown) should complete cleanly even with no server
	if err := exp.Finish(ctx); err != nil {
		t.Errorf("Finish returned error: %v", err)
	}
}
