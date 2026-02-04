package analyzer

import (
	"context"
	"testing"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type mockGitHubProvider struct {
	mock.Mock
}

func (m *mockGitHubProvider) FetchWorkflowRuns(ctx context.Context, baseURL, headSHA string, branch, event string) ([]githubapi.WorkflowRun, error) {
	args := m.Called(ctx, baseURL, headSHA, branch, event)
	return args.Get(0).([]githubapi.WorkflowRun), args.Error(1)
}

func (m *mockGitHubProvider) FetchRepository(ctx context.Context, baseURL string) (*githubapi.RepoMeta, error) {
	args := m.Called(ctx, baseURL)
	return args.Get(0).(*githubapi.RepoMeta), args.Error(1)
}

func (m *mockGitHubProvider) FetchCommitAssociatedPRs(ctx context.Context, owner, repo, sha string) ([]githubapi.PullAssociated, error) {
	args := m.Called(ctx, owner, repo, sha)
	return args.Get(0).([]githubapi.PullAssociated), args.Error(1)
}

func (m *mockGitHubProvider) FetchCommit(ctx context.Context, baseURL, sha string) (*githubapi.CommitResponse, error) {
	args := m.Called(ctx, baseURL, sha)
	return args.Get(0).(*githubapi.CommitResponse), args.Error(1)
}

func (m *mockGitHubProvider) FetchPullRequest(ctx context.Context, baseURL, identifier string) (*githubapi.PullRequest, error) {
	args := m.Called(ctx, baseURL, identifier)
	return args.Get(0).(*githubapi.PullRequest), args.Error(1)
}

func (m *mockGitHubProvider) FetchPRReviews(ctx context.Context, owner, repo, prNumber string) ([]githubapi.Review, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	return args.Get(0).([]githubapi.Review), args.Error(1)
}

func (m *mockGitHubProvider) FetchPRComments(ctx context.Context, owner, repo, prNumber string) ([]githubapi.Review, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	return args.Get(0).([]githubapi.Review), args.Error(1)
}

func (m *mockGitHubProvider) FetchJobsPaginated(ctx context.Context, urlValue string) ([]githubapi.Job, error) {
	args := m.Called(ctx, urlValue)
	return args.Get(0).([]githubapi.Job), args.Error(1)
}

func (m *mockGitHubProvider) FetchBranchProtection(ctx context.Context, owner, repo, branch string) (*githubapi.BranchProtection, error) {
	args := m.Called(ctx, owner, repo, branch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*githubapi.BranchProtection), args.Error(1)
}

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

	emitter := NewTraceEmitter(otel.Tracer("test"))
	mockClient := new(mockGitHubProvider)

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

		emitter.EmitMarkers(ctx, &RawData{
			ReviewEvents: reviewEvents,
		})

		_, err := buildURLResult(ctx, parsed, 0, "sha", "main", "PR 1", "url", reviewEvents, nil, nil, nil, 0, 0, nil, nil, mockClient, nil, 0)
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

		emitter.EmitMarkers(ctx, &RawData{
			CommitTimeMs: &commitTimeMs,
		})

		_, err := buildURLResult(ctx, parsed, 0, "sha123", "main", "Commit sha123", "url", nil, nil, &commitTimeMs, nil, 0, 0, nil, nil, mockClient, nil, 0)
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
