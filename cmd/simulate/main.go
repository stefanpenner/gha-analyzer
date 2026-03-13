package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx := context.Background()
	fmt.Println("Starting End-to-End Simulation...")

	// 1. Setup Exporters (OTel Collector)
	otelExporter, err := otelexport.NewExporter(ctx, "localhost:4318")
	if err != nil {
		fmt.Printf("Failed to create OTel exporter: %v\n", err)
		os.Exit(1)
	}

	pipeline := core.NewPipeline(otelExporter)

	// 2. Simulate a Workflow Run using SpanStubs
	now := time.Now()
	tid := githubapi.NewTraceID(123456789, 1)
	wfSID := githubapi.NewSpanID(123456789)
	wfSC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     wfSID,
		TraceFlags: trace.FlagsSampled,
	})

	fmt.Println("Simulating workflow: 'E2E Test Workflow'...")

	var stubs tracetest.SpanStubs

	// Workflow span
	stubs = append(stubs, tracetest.SpanStub{
		Name:        "Workflow: E2E Test Workflow",
		SpanContext: wfSC,
		StartTime:   now,
		EndTime:     now.Add(200 * time.Millisecond),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "workflow"),
			attribute.String("github.conclusion", "success"),
			attribute.String("github.repository", "stefanpenner/gha-analyzer"),
			attribute.Int64("github.run_id", 123456789),
		},
	})

	// Job 1
	job1SID := githubapi.NewSpanID(1001)
	job1SC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     job1SID,
		TraceFlags: trace.FlagsSampled,
	})
	stubs = append(stubs, tracetest.SpanStub{
		Name:        "Job: Build and Test",
		SpanContext: job1SC,
		Parent:      wfSC,
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "job"),
			attribute.String("github.conclusion", "success"),
			attribute.String("github.job_name", "Build and Test"),
		},
	})

	// Job 2
	job2SID := githubapi.NewSpanID(1002)
	job2SC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     job2SID,
		TraceFlags: trace.FlagsSampled,
	})
	stubs = append(stubs, tracetest.SpanStub{
		Name:        "Job: Lint",
		SpanContext: job2SC,
		Parent:      wfSC,
		StartTime:   now,
		EndTime:     now.Add(50 * time.Millisecond),
		Attributes: []attribute.KeyValue{
			attribute.String("type", "job"),
			attribute.String("github.conclusion", "success"),
			attribute.String("github.job_name", "Lint"),
		},
	})

	// 3. Convert to ReadOnlySpan and process
	spans := stubs.Snapshots()

	fmt.Println("Sending spans to OTel Collector...")
	if err := pipeline.Process(ctx, spans); err != nil {
		fmt.Printf("Processing failed: %v\n", err)
	}

	if err := pipeline.Finish(ctx); err != nil {
		fmt.Printf("Finalizing failed: %v\n", err)
	}

	fmt.Println("Simulation complete! Check Grafana at http://localhost:3000/d/gha-analyzer")
}
