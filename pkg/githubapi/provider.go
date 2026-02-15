package githubapi

import (
	"context"
)

// GitHubProvider defines the interface for interacting with GitHub's API.
type GitHubProvider interface {
	FetchWorkflowRuns(ctx context.Context, baseURL, headSHA string, branch, event string) ([]WorkflowRun, error)
	FetchRecentWorkflowRuns(ctx context.Context, owner, repo string, days int, branch, workflow string, onPage func(fetched, total int)) ([]WorkflowRun, error)
	FetchWorkflowRunsPage(ctx context.Context, owner, repo string, days int, branch string, page int) (*WorkflowRunsResponse, error)
	FetchRepository(ctx context.Context, baseURL string) (*RepoMeta, error)
	FetchCommitAssociatedPRs(ctx context.Context, owner, repo, sha string) ([]PullAssociated, error)
	FetchCommit(ctx context.Context, baseURL, sha string) (*CommitResponse, error)
	FetchPullRequest(ctx context.Context, baseURL, identifier string) (*PullRequest, error)
	FetchPRReviews(ctx context.Context, owner, repo, prNumber string) ([]Review, error)
	FetchPRComments(ctx context.Context, owner, repo, prNumber string) ([]Review, error)
	FetchJobsPaginated(ctx context.Context, urlValue string) ([]Job, error)
	FetchBranchProtection(ctx context.Context, owner, repo, branch string) (*BranchProtection, error)
}
