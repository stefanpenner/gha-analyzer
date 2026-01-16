package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("githubapi")

type Context struct {
	GitHubToken string
	CacheDir    string
}

func NewContext(token string) Context {
	return Context{GitHubToken: token, CacheDir: ".gha-cache"}
}

type Client struct {
	context    Context
	httpClient *http.Client
	semaphore  chan struct{}
	limiter    *rateLimiter
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func WithCacheDir(dir string) Option {
	return func(c *Client) {
		c.context.CacheDir = dir
	}
}

func WithMaxConcurrency(max int) Option {
	return func(c *Client) {
		if max < 1 {
			max = 1
		}
		c.semaphore = make(chan struct{}, max)
	}
}

func NewClient(context Context, opts ...Option) *Client {
	client := &Client{
		context: context,
		limiter: &rateLimiter{},
	}
	for _, opt := range opts {
		opt(client)
	}

	if client.semaphore == nil {
		client.semaphore = make(chan struct{}, 10)
	}

	if client.httpClient == nil {
		// Base transport
		var base http.RoundTripper = http.DefaultTransport

		// Add rate limiting (MUST be behind cache)
		base = &RateLimitedTransport{
			Base:      base,
			Limiter:   client.limiter,
			Semaphore: client.semaphore,
		}

		// Add caching
		if client.context.CacheDir != "" {
			base = NewCachedTransport(base, client.context.CacheDir)
		}

		// Add OTel instrumentation
		base = otelhttp.NewTransport(base)

		client.httpClient = &http.Client{
			Transport: base,
			Timeout:   60 * time.Second,
		}
	}

	return client
}

type WorkflowRunsResponse struct {
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type WorkflowRun struct {
	ID         int64   `json:"id"`
	RunAttempt int64   `json:"run_attempt"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion string  `json:"conclusion"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	Repository RepoRef `json:"repository"`
}

type RepoRef struct {
	Owner RepoOwner `json:"owner"`
	Name  string    `json:"name"`
}

type RepoOwner struct {
	Login string `json:"login"`
}

type JobsResponse struct {
	Jobs []Job `json:"jobs"`
}

type Job struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
	RunnerName  string `json:"runner_name"`
	HTMLURL     string `json:"html_url"`
	Steps       []Step `json:"steps"`
}

type Step struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	Number      int    `json:"number"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type PullRequest struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	Head     PRRef     `json:"head"`
	Base     PRRef     `json:"base"`
	MergedAt *string   `json:"merged_at"`
	MergedBy *UserInfo `json:"merged_by"`
}

type PRRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type Review struct {
	ID          int64    `json:"id"`
	State       string   `json:"state"`
	SubmittedAt string   `json:"submitted_at"`
	User        UserInfo `json:"user"`
	Body        string   `json:"body"`
	HTMLURL     string   `json:"html_url"`
}

type UserInfo struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

type CommitResponse struct {
	Commit CommitDetails `json:"commit"`
}

type CommitDetails struct {
	Committer CommitAuthor `json:"committer"`
	Author    CommitAuthor `json:"author"`
}

type CommitAuthor struct {
	Date string `json:"date"`
}

type RepoMeta struct {
	DefaultBranch string `json:"default_branch"`
}

type PullAssociated struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
	MergedAt *string   `json:"merged_at"`
	MergedBy *UserInfo `json:"merged_by"`
	HTMLURL  string    `json:"html_url"`
}

type GitHubError struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

type rateLimiter struct {
	mu        sync.Mutex
	remaining int
	resetTime time.Time
}

func (r *rateLimiter) waitIfNeeded() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.remaining == 0 && !r.resetTime.IsZero() {
		if time.Until(r.resetTime) > 0 {
			time.Sleep(time.Until(r.resetTime) + time.Second)
		}
	}
}

func (r *rateLimiter) updateFromHeaders(headers http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if remaining := headers.Get("x-ratelimit-remaining"); remaining != "" {
		if value, err := strconv.Atoi(remaining); err == nil {
			r.remaining = value
		}
	}
	if reset := headers.Get("x-ratelimit-reset"); reset != "" {
		if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
			r.resetTime = time.Unix(seconds, 0)
		}
	}
}

type RateLimitedTransport struct {
	Base      http.RoundTripper
	Limiter   *rateLimiter
	Semaphore chan struct{}
}

func (t *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.Semaphore <- struct{}{}
	defer func() { <-t.Semaphore }()

	t.Limiter.waitIfNeeded()
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	t.Limiter.updateFromHeaders(resp.Header)

	if shouldRetryRateLimit(resp) {
		waitForRateLimit(resp)
		_ = resp.Body.Close()
		t.Limiter.waitIfNeeded()
		resp, err = t.Base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		t.Limiter.updateFromHeaders(resp.Header)
	}

	return resp, nil
}

func doRequest(ctx context.Context, client *Client, req *http.Request) (*http.Response, error) {
	return client.httpClient.Do(req.WithContext(ctx))
}

func shouldRetryRateLimit(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("x-ratelimit-remaining") == "0" {
		return true
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	return false
}

func waitForRateLimit(resp *http.Response) {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
			time.Sleep(time.Duration(seconds) * time.Second)
			return
		}
	}
	if reset := resp.Header.Get("x-ratelimit-reset"); reset != "" {
		if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
			resetTime := time.Unix(seconds, 0)
			if time.Until(resetTime) > 0 {
				time.Sleep(time.Until(resetTime) + time.Second)
			}
		}
	}
}

func fetchWithAuth(ctx context.Context, client *Client, urlValue string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlValue, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+client.context.GitHubToken)
	if accept == "" {
		accept = "application/vnd.github.v3+json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Node")

	resp, err := doRequest(ctx, client, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, handleGithubError(resp, urlValue)
	}
	return resp, nil
}

func decodeJSON[T any](resp *http.Response, target *T) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func handleGithubError(response *http.Response, requestURL string) error {
	status := response.StatusCode
	statusText := response.Status

	var bodyText string
	var message string
	var documentationURL string

	contentType := response.Header.Get("content-type")
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(contentType, "application/json") {
		var data GitHubError
		if err := json.Unmarshal(body, &data); err == nil {
			message = data.Message
			documentationURL = data.DocumentationURL
		}
	} else {
		bodyText = string(body)
	}

	ssoHeader := response.Header.Get("x-github-sso")
	oauthScopes := response.Header.Get("x-oauth-scopes")
	acceptedScopes := response.Header.Get("x-accepted-oauth-scopes")
	rateRemaining := response.Header.Get("x-ratelimit-remaining")

	base := fmt.Sprintf("Error fetching %s: %d %s", requestURL, status, statusText)
	detail := message
	if detail == "" {
		detail = bodyText
	}

	if status == 401 {
		lines := []string{
			base + formatDetail(detail),
			"➡️  Authentication failed. Ensure a valid token (env GITHUB_TOKEN or CLI arg).",
			"   - Fine-grained PAT: grant repository access and Read for: Contents, Actions, Pull requests, Checks.",
			"   - Classic PAT: include repo scope for private repos.",
		}
		if documentationURL != "" {
			lines = append(lines, "   Docs: "+documentationURL)
		}
		return errors.New(strings.Join(lines, "\n"))
	}

	if status == 403 {
		if ssoHeader != "" && strings.Contains(strings.ToLower(ssoHeader), "required") {
			lines := []string{
				base + formatDetail(detail),
				"❌ GitHub API request forbidden due to SSO requirement for this token.",
			}
			if ssoURL := extractSSOURL(ssoHeader); ssoURL != "" {
				lines = append(lines, fmt.Sprintf("➡️  Authorize SSO for this token by visiting:\n   %s\nThen re-run the command.", ssoURL))
			} else {
				lines = append(lines, "➡️  Authorize SSO for this token in your organization, then re-run.")
			}
			return errors.New(strings.Join(lines, "\n"))
		}

		if rateRemaining == "0" {
			lines := []string{
				base + formatDetail(detail),
				"➡️  API rate limit reached. Wait for reset or use an authenticated token with higher limits.",
			}
			return errors.New(strings.Join(lines, "\n"))
		}

		lines := []string{
			base + formatDetail(detail),
			"➡️  Permission issue. Verify token access to this repository and required scopes.",
		}
		if acceptedScopes != "" {
			lines = append(lines, fmt.Sprintf("   Required scopes (server hint): %s", acceptedScopes))
		}
		if oauthScopes != "" {
			lines = append(lines, fmt.Sprintf("   Your token scopes: %s", oauthScopes))
		}
		lines = append(lines, "   - Fine-grained PAT: grant repo access and Read for Contents, Actions, Pull requests, Checks.")
		lines = append(lines, "   - Classic PAT: include repo scope for private repos.")
		if documentationURL != "" {
			lines = append(lines, "   Docs: "+documentationURL)
		}
		return errors.New(strings.Join(lines, "\n"))
	}

	if status == 404 {
		lines := []string{
			base + formatDetail(detail),
			"➡️  Not found. On private repos, 404 can indicate insufficient token access. Check repository access and scopes.",
		}
		return errors.New(strings.Join(lines, "\n"))
	}

	if detail != "" {
		return fmt.Errorf("%s - %s", base, detail)
	}
	return errors.New(base)
}

func formatDetail(detail string) string {
	if detail == "" {
		return ""
	}
	return " - " + detail
}

func extractSSOURL(header string) string {
	parts := strings.Split(header, "url=")
	if len(parts) < 2 {
		return ""
	}
	segment := parts[1]
	if i := strings.IndexAny(segment, "; "); i >= 0 {
		return segment[:i]
	}
	return segment
}

func FetchWorkflowRuns(ctx context.Context, client *Client, baseURL, headSHA string, branch, event string) ([]WorkflowRun, error) {
	ctx, span := tracer.Start(ctx, "FetchWorkflowRuns", trace.WithAttributes(
		attribute.String("github.baseURL", baseURL),
		attribute.String("github.headSHA", headSHA),
		attribute.String("github.branch", branch),
		attribute.String("github.event", event),
	))
	defer span.End()

	params := url.Values{}
	params.Set("head_sha", headSHA)
	params.Set("per_page", "100")
	if branch != "" {
		params.Set("branch", branch)
	}
	if event != "" {
		params.Set("event", event)
	}
	runsURL := fmt.Sprintf("%s/actions/runs?%s", baseURL, params.Encode())
	return fetchWorkflowRunsPaginated(ctx, client, runsURL)
}

func FetchRepository(ctx context.Context, client *Client, baseURL string) (*RepoMeta, error) {
	ctx, span := tracer.Start(ctx, "FetchRepository", trace.WithAttributes(
		attribute.String("github.baseURL", baseURL),
	))
	defer span.End()

	resp, err := fetchWithAuth(ctx, client, baseURL, "")
	if err != nil {
		return nil, err
	}
	var repo RepoMeta
	if err := decodeJSON(resp, &repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

func FetchCommitAssociatedPRs(ctx context.Context, client *Client, owner, repo, sha string) ([]PullAssociated, error) {
	ctx, span := tracer.Start(ctx, "FetchCommitAssociatedPRs", trace.WithAttributes(
		attribute.String("github.owner", owner),
		attribute.String("github.repo", repo),
		attribute.String("github.sha", sha),
	))
	defer span.End()

	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/pulls?per_page=100", owner, repo, sha)
	resp, err := fetchWithAuth(ctx, client, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var prs []PullAssociated
	if err := decodeJSON(resp, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

func FetchCommit(ctx context.Context, client *Client, baseURL, sha string) (*CommitResponse, error) {
	ctx, span := tracer.Start(ctx, "FetchCommit", trace.WithAttributes(
		attribute.String("github.baseURL", baseURL),
		attribute.String("github.sha", sha),
	))
	defer span.End()

	commitURL := fmt.Sprintf("%s/commits/%s", baseURL, sha)
	resp, err := fetchWithAuth(ctx, client, commitURL, "")
	if err != nil {
		return nil, err
	}
	var commit CommitResponse
	if err := decodeJSON(resp, &commit); err != nil {
		return nil, err
	}
	return &commit, nil
}

func FetchPullRequest(ctx context.Context, client *Client, baseURL, identifier string) (*PullRequest, error) {
	ctx, span := tracer.Start(ctx, "FetchPullRequest", trace.WithAttributes(
		attribute.String("github.baseURL", baseURL),
		attribute.String("github.identifier", identifier),
	))
	defer span.End()

	prURL := fmt.Sprintf("%s/pulls/%s", baseURL, identifier)
	resp, err := fetchWithAuth(ctx, client, prURL, "")
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err := decodeJSON(resp, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func FetchPRReviews(ctx context.Context, client *Client, owner, repo, prNumber string) ([]Review, error) {
	ctx, span := tracer.Start(ctx, "FetchPRReviews", trace.WithAttributes(
		attribute.String("github.owner", owner),
		attribute.String("github.repo", repo),
		attribute.String("github.prNumber", prNumber),
	))
	defer span.End()

	reviewsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/reviews?per_page=100", owner, repo, prNumber)
	return fetchReviewsPaginated(ctx, client, reviewsURL)
}

func FetchPRComments(ctx context.Context, client *Client, owner, repo, prNumber string) ([]Review, error) {
	ctx, span := tracer.Start(ctx, "FetchPRComments", trace.WithAttributes(
		attribute.String("github.owner", owner),
		attribute.String("github.repo", repo),
		attribute.String("github.prNumber", prNumber),
	))
	defer span.End()

	// Use Review struct for simplicity as they share similar fields (ID, User, Body, SubmittedAt/CreatedAt)
	commentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments?per_page=100", owner, repo, prNumber)
	return fetchCommentsPaginated(ctx, client, commentsURL)
}

func FetchJobsPaginated(ctx context.Context, client *Client, urlValue string) ([]Job, error) {
	ctx, span := tracer.Start(ctx, "FetchJobsPaginated", trace.WithAttributes(
		attribute.String("github.url", urlValue),
	))
	defer span.End()

	var all []Job
	nextURL := urlValue
	for nextURL != "" {
		resp, err := fetchWithAuth(ctx, client, nextURL, "")
		if err != nil {
			return nil, err
		}
		var data JobsResponse
		if err := decodeJSON(resp, &data); err != nil {
			return nil, err
		}
		all = append(all, data.Jobs...)
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

func fetchWorkflowRunsPaginated(ctx context.Context, client *Client, urlValue string) ([]WorkflowRun, error) {
	ctx, span := tracer.Start(ctx, "fetchWorkflowRunsPaginated", trace.WithAttributes(
		attribute.String("github.url", urlValue),
	))
	defer span.End()

	var all []WorkflowRun
	nextURL := urlValue
	for nextURL != "" {
		resp, err := fetchWithAuth(ctx, client, nextURL, "")
		if err != nil {
			return nil, err
		}
		var data WorkflowRunsResponse
		if err := decodeJSON(resp, &data); err != nil {
			return nil, err
		}
		all = append(all, data.WorkflowRuns...)
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

func fetchReviewsPaginated(ctx context.Context, client *Client, urlValue string) ([]Review, error) {
	ctx, span := tracer.Start(ctx, "fetchReviewsPaginated", trace.WithAttributes(
		attribute.String("github.url", urlValue),
	))
	defer span.End()

	var all []Review
	nextURL := urlValue
	for nextURL != "" {
		resp, err := fetchWithAuth(ctx, client, nextURL, "")
		if err != nil {
			return nil, err
		}
		var data []Review
		if err := decodeJSON(resp, &data); err != nil {
			return nil, err
		}
		all = append(all, data...)
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

func fetchCommentsPaginated(ctx context.Context, client *Client, urlValue string) ([]Review, error) {
	ctx, span := tracer.Start(ctx, "fetchCommentsPaginated", trace.WithAttributes(
		attribute.String("github.url", urlValue),
	))
	defer span.End()

	var all []Review
	nextURL := urlValue
	for nextURL != "" {
		resp, err := fetchWithAuth(ctx, client, nextURL, "")
		if err != nil {
			return nil, err
		}
		
		type Comment struct {
			ID        int64    `json:"id"`
			User      UserInfo `json:"user"`
			Body      string   `json:"body"`
			CreatedAt string   `json:"created_at"`
			HTMLURL   string   `json:"html_url"`
		}
		var data []Comment
		if err := decodeJSON(resp, &data); err != nil {
			return nil, err
		}
		for _, c := range data {
			all = append(all, Review{
				ID:          c.ID,
				User:        c.User,
				Body:        c.Body,
				SubmittedAt: c.CreatedAt,
				HTMLURL:     c.HTMLURL,
			})
		}
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}
	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		section := strings.TrimSpace(part)
		if strings.Contains(section, `rel="next"`) {
			start := strings.Index(section, "<")
			end := strings.Index(section, ">")
			if start >= 0 && end > start {
				return section[start+1 : end]
			}
		}
	}
	return ""
}
