package analyzer

import (
	"testing"
	"time"

	"github.com/stefanpenner/otel-analyzer/pkg/githubapi"
	"github.com/stefanpenner/otel-analyzer/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"context"
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

func (m *mockGitHubProvider) FetchRecentWorkflowRuns(ctx context.Context, owner, repo string, days int, branch, workflow string, onPage func(fetched, total int)) ([]githubapi.WorkflowRun, error) {
	args := m.Called(ctx, owner, repo, days, branch, workflow, onPage)
	return args.Get(0).([]githubapi.WorkflowRun), args.Error(1)
}

func (m *mockGitHubProvider) FetchRunTiming(ctx context.Context, owner, repo string, runID int64) (*githubapi.RunTiming, error) {
	args := m.Called(ctx, owner, repo, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*githubapi.RunTiming), args.Error(1)
}

func (m *mockGitHubProvider) FetchCheckRunsForCommit(ctx context.Context, owner, repo, sha string) ([]githubapi.CheckRun, error) {
	args := m.Called(ctx, owner, repo, sha)
	return args.Get(0).([]githubapi.CheckRun), args.Error(1)
}

func (m *mockGitHubProvider) FetchAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]githubapi.Annotation, error) {
	args := m.Called(ctx, owner, repo, checkRunID)
	return args.Get(0).([]githubapi.Annotation), args.Error(1)
}

func (m *mockGitHubProvider) ListArtifacts(ctx context.Context, owner, repo string, runID int64) ([]githubapi.Artifact, error) {
	args := m.Called(ctx, owner, repo, runID)
	return args.Get(0).([]githubapi.Artifact), args.Error(1)
}

func (m *mockGitHubProvider) DownloadArtifact(ctx context.Context, url string) ([]byte, error) {
	args := m.Called(ctx, url)
	return args.Get(0).([]byte), args.Error(1)
}

func TestWorkflowQueueTimeSpan(t *testing.T) {
	t.Run("emits workflow queue span when RunStartedAt is after CreatedAt", func(t *testing.T) {
		mockClient := new(mockGitHubProvider)
		builder := &SpanBuilder{}

		run := githubapi.WorkflowRun{
			ID:           100,
			RunAttempt:   1,
			Name:         "CI",
			Status:       "completed",
			Conclusion:   "success",
			CreatedAt:    "2026-03-18T17:17:33Z",
			RunStartedAt: "2026-03-18T17:47:58Z",
			UpdatedAt:    "2026-03-18T18:50:30Z",
			HeadSHA:      "abc123",
			Repository: githubapi.RepoRef{
				Owner: githubapi.RepoOwner{Login: "owner"},
				Name:  "repo",
			},
		}

		job := githubapi.Job{
			ID:          200,
			Name:        "Build",
			Status:      "completed",
			Conclusion:  "success",
			CreatedAt:   "2026-03-18T17:47:58Z",
			StartedAt:   "2026-03-18T17:56:49Z",
			CompletedAt: "2026-03-18T18:47:19Z",
			RunnerName:  "runner-1",
		}

		jobsURL := "https://api.github.com/repos/owner/repo/actions/runs/100/jobs?per_page=100"
		mockClient.On("FetchJobsPaginated", mock.Anything, jobsURL).Return([]githubapi.Job{job}, nil)
		mockClient.On("FetchCheckRunsForCommit", mock.Anything, "owner", "repo", "abc123").Return([]githubapi.CheckRun{}, nil)
		mockClient.On("FetchRunTiming", mock.Anything, "owner", "repo", int64(100)).Return((*githubapi.RunTiming)(nil), nil)
		mockClient.On("ListArtifacts", mock.Anything, "owner", "repo", int64(100)).Return([]githubapi.Artifact{}, nil)

		createdAt, _ := utils.ParseTime(run.CreatedAt)
		earliestTime := createdAt.UnixMilli()

		_, traceEvents, _, _, err := processWorkflowRun(
			context.Background(), run, 0, 1001, earliestTime,
			"owner", "repo", "1", 0, "https://github.com/owner/repo/pull/1", "pr",
			nil, mockClient, nil, builder, NewTraceEmitter(builder), AnalyzeOptions{NoArtifacts: true},
		)
		assert.NoError(t, err)

		// Check trace events for workflow queue span
		var wfQueueFound bool
		for _, event := range traceEvents {
			if event.Cat == "queued" && event.Args["type"] == "workflow_queued" {
				wfQueueFound = true
				queueMs := event.Args["queue_time_ms"].(int64)
				// 17:47:58 - 17:17:33 = 30m25s = 1825000ms
				assert.Equal(t, int64(1825000), queueMs)
				assert.Equal(t, int64(0), event.Ts, "should start at normalized time 0 (earliest)")
			}
		}
		assert.True(t, wfQueueFound, "Workflow queue trace event not found")

		// Check OTel spans for workflow queue span
		spans := builder.Spans()
		var otelQueueFound bool
		for _, s := range spans {
			if s.Name() == "⏳ Workflow Queued" {
				otelQueueFound = true
				attrs := map[string]interface{}{}
				for _, a := range s.Attributes() {
					attrs[string(a.Key)] = a.Value.AsInterface()
				}
				assert.Equal(t, "workflow_queued", attrs["type"])
				assert.Equal(t, int64(1825000), attrs["queue_time_ms"])

				expectedStart, _ := utils.ParseTime("2026-03-18T17:17:33Z")
				expectedEnd, _ := utils.ParseTime("2026-03-18T17:47:58Z")
				assert.Equal(t, expectedStart, s.StartTime())
				assert.Equal(t, expectedEnd, s.EndTime())
			}
		}
		assert.True(t, otelQueueFound, "Workflow queue OTel span not found")
	})

	t.Run("no queue span when RunStartedAt equals CreatedAt", func(t *testing.T) {
		mockClient := new(mockGitHubProvider)
		builder := &SpanBuilder{}

		run := githubapi.WorkflowRun{
			ID:           101,
			RunAttempt:   1,
			Name:         "CI",
			Status:       "completed",
			Conclusion:   "success",
			CreatedAt:    "2026-03-18T17:17:33Z",
			RunStartedAt: "2026-03-18T17:17:33Z",
			UpdatedAt:    "2026-03-18T17:30:00Z",
			HeadSHA:      "abc123",
			Repository: githubapi.RepoRef{
				Owner: githubapi.RepoOwner{Login: "owner"},
				Name:  "repo",
			},
		}

		job := githubapi.Job{
			ID:          201,
			Name:        "Build",
			Status:      "completed",
			Conclusion:  "success",
			CreatedAt:   "2026-03-18T17:17:33Z",
			StartedAt:   "2026-03-18T17:17:40Z",
			CompletedAt: "2026-03-18T17:30:00Z",
			RunnerName:  "runner-1",
		}

		jobsURL := "https://api.github.com/repos/owner/repo/actions/runs/101/jobs?per_page=100"
		mockClient.On("FetchJobsPaginated", mock.Anything, jobsURL).Return([]githubapi.Job{job}, nil)
		mockClient.On("FetchCheckRunsForCommit", mock.Anything, "owner", "repo", "abc123").Return([]githubapi.CheckRun{}, nil)
		mockClient.On("FetchRunTiming", mock.Anything, "owner", "repo", int64(101)).Return((*githubapi.RunTiming)(nil), nil)
		mockClient.On("ListArtifacts", mock.Anything, "owner", "repo", int64(101)).Return([]githubapi.Artifact{}, nil)

		createdAt, _ := utils.ParseTime(run.CreatedAt)
		earliestTime := createdAt.UnixMilli()

		_, traceEvents, _, _, err := processWorkflowRun(
			context.Background(), run, 0, 1001, earliestTime,
			"owner", "repo", "1", 0, "https://github.com/owner/repo/pull/1", "pr",
			nil, mockClient, nil, builder, NewTraceEmitter(builder), AnalyzeOptions{NoArtifacts: true},
		)
		assert.NoError(t, err)

		for _, event := range traceEvents {
			if event.Args != nil && event.Args["type"] == "workflow_queued" {
				t.Fatal("Should not emit workflow queue span when RunStartedAt == CreatedAt")
			}
		}
	})

	t.Run("no queue span when RunStartedAt is empty", func(t *testing.T) {
		mockClient := new(mockGitHubProvider)
		builder := &SpanBuilder{}

		run := githubapi.WorkflowRun{
			ID:           102,
			RunAttempt:   1,
			Name:         "CI",
			Status:       "completed",
			Conclusion:   "success",
			CreatedAt:    "2026-03-18T17:17:33Z",
			RunStartedAt: "",
			UpdatedAt:    "2026-03-18T17:30:00Z",
			HeadSHA:      "abc123",
			Repository: githubapi.RepoRef{
				Owner: githubapi.RepoOwner{Login: "owner"},
				Name:  "repo",
			},
		}

		job := githubapi.Job{
			ID:          202,
			Name:        "Build",
			Status:      "completed",
			Conclusion:  "success",
			CreatedAt:   "2026-03-18T17:17:33Z",
			StartedAt:   "2026-03-18T17:17:40Z",
			CompletedAt: "2026-03-18T17:30:00Z",
			RunnerName:  "runner-1",
		}

		jobsURL := "https://api.github.com/repos/owner/repo/actions/runs/102/jobs?per_page=100"
		mockClient.On("FetchJobsPaginated", mock.Anything, jobsURL).Return([]githubapi.Job{job}, nil)
		mockClient.On("FetchCheckRunsForCommit", mock.Anything, "owner", "repo", "abc123").Return([]githubapi.CheckRun{}, nil)
		mockClient.On("FetchRunTiming", mock.Anything, "owner", "repo", int64(102)).Return((*githubapi.RunTiming)(nil), nil)
		mockClient.On("ListArtifacts", mock.Anything, "owner", "repo", int64(102)).Return([]githubapi.Artifact{}, nil)

		createdAt, _ := utils.ParseTime(run.CreatedAt)
		earliestTime := createdAt.UnixMilli()

		_, traceEvents, _, _, err := processWorkflowRun(
			context.Background(), run, 0, 1001, earliestTime,
			"owner", "repo", "1", 0, "https://github.com/owner/repo/pull/1", "pr",
			nil, mockClient, nil, builder, NewTraceEmitter(builder), AnalyzeOptions{NoArtifacts: true},
		)
		assert.NoError(t, err)

		for _, event := range traceEvents {
			if event.Args != nil && event.Args["type"] == "workflow_queued" {
				t.Fatal("Should not emit workflow queue span when RunStartedAt is empty")
			}
		}
	})
}

func TestSpanBuilderGeneration(t *testing.T) {
	mockClient := new(mockGitHubProvider)

	t.Run("Review markers emit correct spans", func(t *testing.T) {
		builder := &SpanBuilder{}
		emitter := NewTraceEmitter(builder)

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

		emitter.EmitMarkers(&RawData{
			ReviewEvents: reviewEvents,
		}, 0)

		_, err := buildURLResult(context.Background(), parsed, 0, "sha", "main", "PR 1", "url", reviewEvents, nil, nil, nil, 0, 0, nil, nil, mockClient, nil, 0, builder, AnalyzeOptions{})
		assert.NoError(t, err)

		spans := builder.Spans()

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
		builder := &SpanBuilder{}
		emitter := NewTraceEmitter(builder)

		commitTime := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
		commitTimeMs := commitTime.UnixMilli()

		parsed := utils.ParsedGitHubURL{
			Owner:      "nodejs",
			Repo:       "node",
			Type:       "commit",
			Identifier: "sha123",
		}

		emitter.EmitMarkers(&RawData{
			CommitTimeMs: &commitTimeMs,
		}, 0)

		_, err := buildURLResult(context.Background(), parsed, 0, "sha123", "main", "Commit sha123", "url", nil, nil, &commitTimeMs, nil, 0, 0, nil, nil, mockClient, nil, 0, builder, AnalyzeOptions{})
		assert.NoError(t, err)

		spans := builder.Spans()

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
