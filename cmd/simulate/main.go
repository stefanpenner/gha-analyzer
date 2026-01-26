package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
	otelexport "github.com/stefanpenner/gha-analyzer/pkg/export/otel"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	ctx := context.Background()
	fmt.Println("🧪 Starting End-to-End Simulation...")

	// 1. Setup OTel with local collector
	collector := core.NewSpanCollector()
	res, _ := otelexport.GetResource(ctx)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(collector),
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(githubapi.GHIDGenerator{}),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(ctx)

	// 2. Setup Exporters (Local Collector)
	otelExporter, err := otelexport.NewExporter(ctx, "localhost:4318")
	if err != nil {
		fmt.Printf("❌ Failed to create OTel exporter: %v\n", err)
		os.Exit(1)
	}

	pipeline := core.NewPipeline(otelExporter)

	// 3. Simulate a Workflow Run
	tracer := otel.Tracer("simulation")
	
	fmt.Println("📡 Simulating workflow: 'E2E Test Workflow'...")
	ctx, workflowSpan := tracer.Start(ctx, "Workflow: E2E Test Workflow", trace.WithAttributes(
		attribute.String("type", "workflow"),
		attribute.String("github.conclusion", "success"),
		attribute.String("github.repository", "stefanpenner/gha-analyzer"),
		attribute.Int64("github.run_id", 123456789),
	))

	// Simulate Job 1
	_, job1Span := tracer.Start(ctx, "Job: Build and Test", trace.WithAttributes(
		attribute.String("type", "job"),
		attribute.String("github.conclusion", "success"),
		attribute.String("github.job_name", "Build and Test"),
	))
	time.Sleep(100 * time.Millisecond)
	job1Span.End()

	// Simulate Job 2 (Parallel)
	_, job2Span := tracer.Start(ctx, "Job: Lint", trace.WithAttributes(
		attribute.String("type", "job"),
		attribute.String("github.conclusion", "success"),
		attribute.String("github.job_name", "Lint"),
	))
	time.Sleep(50 * time.Millisecond)
	job2Span.End()

	workflowSpan.End()

	// 4. Flush and Process
	fmt.Println("📤 Flushing spans to OTel Collector...")
	tp.ForceFlush(ctx)
	spans := collector.Spans()

	if err := pipeline.Process(ctx, spans); err != nil {
		fmt.Printf("❌ Processing failed: %v\n", err)
		os.Exit(1)
	}

	if err := pipeline.Finish(ctx); err != nil {
		fmt.Printf("❌ Finalizing failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Simulation complete! Check Grafana at http://localhost:3000/d/gha-analyzer")
}
