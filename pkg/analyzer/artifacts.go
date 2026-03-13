package analyzer

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/ingest/otlpfile"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// IngestTraceArtifacts downloads artifacts matching "gha-trace.*" from a workflow run,
// parses them as OTLP JSON or Chrome trace format, and adds spans to the builder.
func IngestTraceArtifacts(ctx context.Context, client githubapi.GitHubProvider, owner, repo string, runID int64, builder *SpanBuilder) error {
	artifacts, err := client.ListArtifacts(ctx, owner, repo, runID)
	if err != nil {
		return nil // best-effort
	}

	for _, artifact := range artifacts {
		if artifact.Expired {
			continue
		}
		if !strings.HasPrefix(artifact.Name, "gha-trace") {
			continue
		}

		data, err := client.DownloadArtifact(ctx, artifact.ArchiveDownloadURL)
		if err != nil {
			continue // best-effort
		}

		spans, err := extractSpansFromZip(data)
		if err != nil {
			continue // best-effort
		}

		// Convert ReadOnlySpans to SpanStubs and add to builder
		for _, s := range spans {
			builder.Add(tracetest.SpanStub{
				Name:        s.Name(),
				SpanContext: s.SpanContext(),
				Parent:      s.Parent(),
				SpanKind:    s.SpanKind(),
				StartTime:   s.StartTime(),
				EndTime:     s.EndTime(),
				Attributes:  s.Attributes(),
				Events:      s.Events(),
				Links:       s.Links(),
				Status:      s.Status(),
			})
		}
	}

	return nil
}

// extractSpansFromZip unzips artifact data and parses any JSON files as OTLP or Chrome traces.
func extractSpansFromZip(data []byte) ([]sdktrace.ReadOnlySpan, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	var allSpans []sdktrace.ReadOnlySpan
	for _, f := range reader.File {
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		spans, err := otlpfile.Parse(rc)
		rc.Close()
		if err != nil || len(spans) == 0 {
			continue
		}
		allSpans = append(allSpans, spans...)
	}

	return allSpans, nil
}
