package analyzer

import (
	"context"
	"testing"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestOTelSpanGeneration(t *testing.T) {
	ctx := context.Background()

	// Use a syncer for tests to avoid race conditions and delays
	collector := core.NewSpanCollector()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(collector),
		sdktrace.WithResource(resource.Empty()),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(ctx)

	t.Run("Review markers emit correct OTel spans", func(t *testing.T) {
		collector.Reset()

		reviewEvents := []ReviewEvent{
			{
				Type:     "review",
				State:    "APPROVED",
				Time:     "2026-01-15T10:00:00Z",
				Reviewer: "stefanpenner",
				URL:      "https://github.com/pull/1#review-1",
			},
		}

		parsed := utils.ParsedGitHubURL{
			Owner:      "nodejs",
			Repo:       "node",
			Type:       "pr",
			Identifier: "1",
		}

		_, err := buildURLResult(ctx, parsed, 0, "sha", "main", "PR 1", "url", reviewEvents, nil, nil, 0, 0, nil, nil, nil, 0)
		assert.NoError(t, err)

		spans := collector.Spans()

		var approvalFound bool
		for _, s := range spans {
			attrs := make(map[string]string)
			for _, a := range s.Attributes() {
				attrs[string(a.Key)] = a.Value.AsString()
			}

			if attrs["type"] == "marker" && attrs["github.event_type"] == "approved" {
				approvalFound = true
				assert.Equal(t, "Review: APPROVED", s.Name())
				assert.Equal(t, "stefanpenner", attrs["github.user"])
			}
		}
		assert.True(t, approvalFound, "Approval marker span not found")
	})

	t.Run("Commit markers are emitted when commitTimeMs is present", func(t *testing.T) {
		collector.Reset()

		commitTime := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
		commitTimeMs := commitTime.UnixMilli()

		parsed := utils.ParsedGitHubURL{
			Owner:      "nodejs",
			Repo:       "node",
			Type:       "commit",
			Identifier: "sha123",
		}

		_, err := buildURLResult(ctx, parsed, 0, "sha123", "main", "Commit sha123", "url", nil, nil, &commitTimeMs, 0, 0, nil, nil, nil, 0)
		assert.NoError(t, err)

		spans := collector.Spans()

		var commitFound bool
		for _, s := range spans {
			if s.Name() == "Commit Created" {
				commitFound = true
				attrs := make(map[string]string)
				for _, a := range s.Attributes() {
					attrs[string(a.Key)] = a.Value.AsString()
				}
				assert.Equal(t, "marker", attrs["type"])
				assert.Equal(t, "commit", attrs["github.event_type"])
			}
		}
		assert.True(t, commitFound, "Commit marker span not found")
	})
}
