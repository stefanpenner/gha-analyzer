package analyzer

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var analyzerTracer = otel.Tracer("analyzer")

type ProgressReporter interface {
	StartURL(urlIndex int, url string)
	SetURLRuns(runCount int)
	SetPhase(phase string)
	SetDetail(detail string)
	ProcessRun()
	Finish()
}

type URLError struct {
	URL string
	Err error
}

func (e URLError) Error() string {
	return fmt.Sprintf("Error processing URL %s: %s", e.URL, e.Err.Error())
}

type AnalyzeOptions struct {
}

func AnalyzeURLs(ctx context.Context, urls []string, client *githubapi.Client, reporter ProgressReporter, opts AnalyzeOptions) ([]URLResult, []TraceEvent, int64, int64, []URLError) {
	allTraceEvents := []TraceEvent{}
	allJobStartTimes := []JobEvent{}
	allJobEndTimes := []JobEvent{}
	urlResults := []URLResult{}
	globalEarliestTime := int64(1<<63 - 1)
	globalLatestTime := int64(0)
	urlErrors := []URLError{}

	for urlIndex, githubURL := range urls {
		if reporter != nil {
			reporter.StartURL(urlIndex, githubURL)
		}
		result, err := processURL(ctx, githubURL, urlIndex, client, reporter, opts)
		if err != nil {
			urlErrors = append(urlErrors, URLError{URL: githubURL, Err: err})
			continue
		}
		if result == nil {
			continue
		}
		urlResults = append(urlResults, *result)
		allTraceEvents = append(allTraceEvents, result.TraceEvents...)
		allJobStartTimes = append(allJobStartTimes, result.JobStartTimes...)
		allJobEndTimes = append(allJobEndTimes, result.JobEndTimes...)

		if result.EarliestTime < globalEarliestTime {
			globalEarliestTime = result.EarliestTime
		}
		// Calculate the actual latest time from all events in this result
		urlLatest := result.EarliestTime
		for _, job := range result.Metrics.JobTimeline {
			if job.EndTime > urlLatest {
				urlLatest = job.EndTime
			}
		}
		for _, event := range result.ReviewEvents {
			ms := event.TimeMillis()
			if ms > urlLatest {
				urlLatest = ms
			}
		}
		if urlLatest > globalLatestTime {
			globalLatestTime = urlLatest
		}
	}

	if reporter != nil {
		reporter.Finish()
	}

	if len(urlResults) == 0 {
		return nil, nil, 0, 0, urlErrors
	}

	GenerateConcurrencyCounters(allJobStartTimes, allJobEndTimes, &allTraceEvents, globalEarliestTime)
	addReviewMarkersToTrace(urlResults, &allTraceEvents)

	combinedTrace := append([]TraceEvent{}, allTraceEvents...)
	allTraceEvents = combinedTrace
	return urlResults, allTraceEvents, globalEarliestTime, globalLatestTime, urlErrors
}

func processURL(ctx context.Context, githubURL string, urlIndex int, client *githubapi.Client, reporter ProgressReporter, opts AnalyzeOptions) (*URLResult, error) {
	parsed, err := utils.ParseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	if reporter != nil {
		reporter.SetPhase("Parsing URL")
		reporter.SetDetail(fmt.Sprintf("%s/%s", parsed.Owner, parsed.Repo))
	}
	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", parsed.Owner, parsed.Repo)

	var headSHA, branchName, displayName, displayURL string
	var reviewEvents []ReviewEvent
	var mergedAtMs *int64
	var commitTimeMs *int64
	var runs []githubapi.WorkflowRun
	allCommitRunsCount := 0
	var allCommitRunsComputeMs int64

	if parsed.Type == "pr" {
		if reporter != nil {
			reporter.SetPhase("Fetching PR metadata")
			reporter.SetDetail(parsed.Identifier)
		}
		analyzingPRURL := fmt.Sprintf("https://github.com/%s/%s/pull/%s", parsed.Owner, parsed.Repo, parsed.Identifier)
		prData, err := githubapi.FetchPullRequest(ctx, client, baseURL, parsed.Identifier)
		if err != nil {
			return nil, err
		}
		if prData.Head.Ref == "" || prData.Head.SHA == "" {
			return nil, fmt.Errorf("Invalid PR response - missing head or base information")
		}
		headSHA = prData.Head.SHA
		branchName = prData.Head.Ref
		displayName = fmt.Sprintf("PR #%s", parsed.Identifier)
		displayURL = analyzingPRURL

		if reporter != nil {
			reporter.SetPhase("Fetching PR reviews and comments")
			reporter.SetDetail(parsed.Identifier)
		}

		reviews, err := githubapi.FetchPRReviews(ctx, client, parsed.Owner, parsed.Repo, parsed.Identifier)
		if err != nil {
			return nil, err
		}
		for _, review := range reviews {
			reviewEvents = append(reviewEvents, ReviewEvent{
				Type:     "review",
				State:    review.State,
				Time:     review.SubmittedAt,
				Reviewer: review.User.Login,
				URL:      firstNonEmpty(review.HTMLURL, analyzingPRURL),
			})
		}

		comments, err := githubapi.FetchPRComments(ctx, client, parsed.Owner, parsed.Repo, parsed.Identifier)
		if err == nil {
			for _, comment := range comments {
				reviewEvents = append(reviewEvents, ReviewEvent{
					Type:     "comment",
					Time:     comment.SubmittedAt,
					Reviewer: comment.User.Login,
					URL:      firstNonEmpty(comment.HTMLURL, analyzingPRURL),
				})
			}
		}

		if prData.MergedAt != nil && *prData.MergedAt != "" {
			reviewEvents = append(reviewEvents, ReviewEvent{
				Type:     "merged",
				Time:     *prData.MergedAt,
				MergedBy: resolvedUser(prData.MergedBy),
				URL:      analyzingPRURL,
				PRNumber: prData.Number,
				PRTitle:  prData.Title,
			})
			if t, ok := utils.ParseTime(*prData.MergedAt); ok {
				ms := t.UnixMilli()
				mergedAtMs = &ms
			}
		}

		if reporter != nil {
			reporter.SetPhase("Fetching workflow runs")
			reporter.SetDetail(headSHA)
		}
		runs, err = githubapi.FetchWorkflowRuns(ctx, client, baseURL, headSHA, "", "")
		if err != nil {
			return nil, err
		}
	} else {
		analyzingCommitURL := fmt.Sprintf("https://github.com/%s/%s/commit/%s", parsed.Owner, parsed.Repo, parsed.Identifier)
		headSHA = parsed.Identifier
		displayName = fmt.Sprintf("commit %s", headSHA[:minInt(8, len(headSHA))])
		displayURL = analyzingCommitURL

		targetBranch := ""
		if reporter != nil {
			reporter.SetPhase("Resolving commit branch")
			reporter.SetDetail(headSHA)
		}
		prs, err := githubapi.FetchCommitAssociatedPRs(ctx, client, parsed.Owner, parsed.Repo, headSHA)
		if err == nil && len(prs) > 0 {
			targetBranch = prs[0].Base.Ref
			
			// Fetch reviews and comments for the first associated PR
			prNumber := fmt.Sprintf("%d", prs[0].Number)
			if reporter != nil {
				reporter.SetPhase("Fetching associated PR metadata")
				reporter.SetDetail(prNumber)
			}
			
			reviews, err := githubapi.FetchPRReviews(ctx, client, parsed.Owner, parsed.Repo, prNumber)
			if err == nil {
				for _, review := range reviews {
					reviewEvents = append(reviewEvents, ReviewEvent{
						Type:     "review",
						State:    review.State,
						Time:     review.SubmittedAt,
						Reviewer: review.User.Login,
						URL:      firstNonEmpty(review.HTMLURL, prs[0].HTMLURL),
					})
				}
			}

			comments, err := githubapi.FetchPRComments(ctx, client, parsed.Owner, parsed.Repo, prNumber)
			if err == nil {
				for _, comment := range comments {
					reviewEvents = append(reviewEvents, ReviewEvent{
						Type:     "comment",
						Time:     comment.SubmittedAt,
						Reviewer: comment.User.Login,
						URL:      firstNonEmpty(comment.HTMLURL, prs[0].HTMLURL),
					})
				}
			}
			
			if prs[0].MergedAt != nil && *prs[0].MergedAt != "" {
				reviewEvents = append(reviewEvents, ReviewEvent{
					Type:     "merged",
					Time:     *prs[0].MergedAt,
					MergedBy: resolvedUser(prs[0].MergedBy),
					URL:      prs[0].HTMLURL,
					PRNumber: prs[0].Number,
					PRTitle:  prs[0].Title,
				})
				if t, ok := utils.ParseTime(*prs[0].MergedAt); ok {
					ms := t.UnixMilli()
					mergedAtMs = &ms
				}
			}
		}
		if targetBranch == "" {
			if repoMeta, err := githubapi.FetchRepository(ctx, client, baseURL); err == nil && repoMeta.DefaultBranch != "" {
				targetBranch = repoMeta.DefaultBranch
			}
		}
		branchName = targetBranch
		if branchName == "" {
			branchName = "unknown"
		}

		if reporter != nil {
			reporter.SetPhase("Fetching commit runs")
			reporter.SetDetail(headSHA)
		}
		allRunsForHead, err := githubapi.FetchWorkflowRuns(ctx, client, baseURL, headSHA, "", "")
		if err != nil {
			return nil, err
		}
		allCommitRunsCount = len(allRunsForHead)

		runs, err = githubapi.FetchWorkflowRuns(ctx, client, baseURL, headSHA, branchName, "push")
		if err != nil {
			return nil, err
		}
		if reporter != nil {
			reporter.SetPhase("Fetching commit metadata")
			reporter.SetDetail(headSHA)
		}
		commitMeta, err := githubapi.FetchCommit(ctx, client, baseURL, headSHA)
		if err == nil {
			dateStr := commitMeta.Commit.Committer.Date
			if dateStr == "" {
				dateStr = commitMeta.Commit.Author.Date
			}
			if t, ok := utils.ParseTime(dateStr); ok {
				ms := t.UnixMilli()
				commitTimeMs = &ms
			}
		}

		if commitTimeMs != nil {
			filtered := []githubapi.WorkflowRun{}
			for _, run := range runs {
				if t, ok := utils.ParseTime(run.CreatedAt); ok {
					if t.UnixMilli() >= *commitTimeMs {
						filtered = append(filtered, run)
					}
				}
			}
			runs = filtered
		}

		if reporter != nil {
			reporter.SetPhase("Computing commit job durations")
			reporter.SetDetail(fmt.Sprintf("%d runs", len(allRunsForHead)))
		}

		// Parallelize fetching jobs for all runs to calculate total compute time
		var computeMu sync.Mutex
		var computeWg sync.WaitGroup
		computeSemaphore := make(chan struct{}, 10) // Higher concurrency for this part
		baseRepoURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", parsed.Owner, parsed.Repo)

		for _, run := range allRunsForHead {
			if run.Status != "completed" {
				continue
			}
			computeWg.Add(1)
			go func(r githubapi.WorkflowRun) {
				defer computeWg.Done()
				computeSemaphore <- struct{}{}
				defer func() { <-computeSemaphore }()

				jobsURL := fmt.Sprintf("%s/actions/runs/%d/jobs?per_page=100", baseRepoURL, r.ID)
				jobs, err := githubapi.FetchJobsPaginated(ctx, client, jobsURL)
				if err != nil {
					return
				}

				var runCompute int64
				for _, job := range jobs {
					if start, ok := utils.ParseTime(job.StartedAt); ok {
						if end, ok := utils.ParseTime(job.CompletedAt); ok {
							if end.After(start) {
								runCompute += end.Sub(start).Milliseconds()
							}
						}
					}
				}
				computeMu.Lock()
				allCommitRunsComputeMs += runCompute
				computeMu.Unlock()
			}(run)
		}
		computeWg.Wait()
	}

	if len(runs) == 0 {
		return nil, nil
	}

	// Calculate urlEarliestTime here to ensure it's consistent
	urlEarliestTime := FindEarliestTimestamp(runs)
	if commitTimeMs != nil && *commitTimeMs < urlEarliestTime {
		urlEarliestTime = *commitTimeMs
	}
	for _, event := range reviewEvents {
		ms := event.TimeMillis()
		if ms < urlEarliestTime {
			urlEarliestTime = ms
		}
	}

	return buildURLResult(ctx, parsed, urlIndex, headSHA, branchName, displayName, displayURL, reviewEvents, mergedAtMs, commitTimeMs, allCommitRunsCount, allCommitRunsComputeMs, runs, client, reporter, urlEarliestTime)
}

func buildURLResult(ctx context.Context, parsed utils.ParsedGitHubURL, urlIndex int, headSHA, branchName, displayName, displayURL string, reviewEvents []ReviewEvent, mergedAtMs, commitTimeMs *int64, allCommitRunsCount int, allCommitRunsComputeMs int64, runs []githubapi.WorkflowRun, client *githubapi.Client, reporter ProgressReporter, urlEarliestTime int64) (*URLResult, error) {
	if reporter != nil {
		reporter.SetURLRuns(len(runs))
		reporter.SetPhase("Processing workflow runs")
		reporter.SetDetail(fmt.Sprintf("%d runs", len(runs)))
	}
	metrics := InitializeMetrics()
	traceEvents := []TraceEvent{}
	jobStartTimes := []JobEvent{}
	jobEndTimes := []JobEvent{}
	
	type runResult struct {
		metrics     Metrics
		traceEvents []TraceEvent
		jobStarts   []JobEvent
		jobEnds     []JobEvent
		err         error
	}

	workerCount := minInt(runtime.GOMAXPROCS(0), len(runs))
	if workerCount == 0 {
		workerCount = 1
	}

	jobsCh := make(chan struct {
		index int
		run   githubapi.WorkflowRun
	})
	resultsCh := make(chan runResult, len(runs))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				processID := (urlIndex+1)*1000 + job.index + 1
				runMetrics, runTrace, runStarts, runEnds, err := processWorkflowRun(ctx, job.run, job.index, processID, urlEarliestTime, parsed.Owner, parsed.Repo, parsed.Identifier, urlIndex, displayURL, parsed.Type, client, reporter)
				resultsCh <- runResult{
					metrics:     runMetrics,
					traceEvents: runTrace,
					jobStarts:   runStarts,
					jobEnds:     runEnds,
					err:         err,
				}
			}
		}()
	}

	for runIndex, run := range runs {
		jobsCh <- struct {
			index int
			run   githubapi.WorkflowRun
		}{index: runIndex, run: run}
	}
	close(jobsCh)
	wg.Wait()
	close(resultsCh)

	for result := range resultsCh {
		if result.err != nil {
			return nil, result.err
		}
		mergeMetrics(&metrics, result.metrics)
		traceEvents = append(traceEvents, result.traceEvents...)
		jobStartTimes = append(jobStartTimes, result.jobStarts...)
		jobEndTimes = append(jobEndTimes, result.jobEnds...)
		if reporter != nil {
			reporter.ProcessRun()
		}
	}

	// Emit OTel spans for review and merge events
	for _, event := range reviewEvents {
		eventTime, _ := utils.ParseTime(event.Time)
		name := "Marker"
		eventType := event.Type
		
		switch event.Type {
		case "review":
			name = fmt.Sprintf("Review: %s", event.State)
			eventType = strings.ToLower(event.State)
		case "comment":
			name = "Comment"
		case "merged":
			name = "Merged"
			if event.PRNumber != 0 {
				name = fmt.Sprintf("Merged PR #%d: %s", event.PRNumber, event.PRTitle)
			}
		}

		_, span := analyzerTracer.Start(ctx, name,
			trace.WithTimestamp(eventTime),
			trace.WithAttributes(
				attribute.String("type", "marker"),
				attribute.String("github.event_type", eventType),
				attribute.String("github.user", firstNonEmpty(event.Reviewer, event.MergedBy)),
				attribute.String("github.url", event.URL),
				attribute.String("github.event_id", fmt.Sprintf("%s-%s-%s-%s", event.Type, event.Time, firstNonEmpty(event.Reviewer, event.MergedBy), event.URL)),
				attribute.String("github.event_time", event.Time),
			),
		)
		span.End(trace.WithTimestamp(eventTime.Add(time.Millisecond)))
	}

	if commitTimeMs != nil {
		t := time.UnixMilli(*commitTimeMs)
		_, span := analyzerTracer.Start(ctx, "Commit Created",
			trace.WithTimestamp(t),
			trace.WithAttributes(
				attribute.String("type", "marker"),
				attribute.String("github.event_type", "commit"),
			),
		)
		span.End(trace.WithTimestamp(t.Add(time.Millisecond)))
	}

	finalMetrics := CalculateFinalMetrics(metrics, len(runs), jobStartTimes, jobEndTimes)
	result := URLResult{
		Owner:                  parsed.Owner,
		Repo:                   parsed.Repo,
		Identifier:             parsed.Identifier,
		BranchName:             branchName,
		HeadSHA:                headSHA,
		Metrics:                finalMetrics,
		TraceEvents:            traceEvents,
		Type:                   parsed.Type,
		DisplayName:            displayName,
		DisplayURL:             displayURL,
		URLIndex:               urlIndex,
		JobStartTimes:          jobStartTimes,
		JobEndTimes:            jobEndTimes,
		EarliestTime:           urlEarliestTime,
		ReviewEvents:           reviewEvents,
		MergedAtMs:             mergedAtMs,
		CommitTimeMs:           commitTimeMs,
		AllCommitRunsCount:     allCommitRunsCount,
		AllCommitRunsComputeMs: allCommitRunsComputeMs,
	}
	return &result, nil
}

func processWorkflowRun(ctx context.Context, run githubapi.WorkflowRun, runIndex, processID int, earliestTime int64, owner, repo, identifier string, urlIndex int, displayURL, sourceType string, client *githubapi.Client, reporter ProgressReporter) (Metrics, []TraceEvent, []JobEvent, []JobEvent, error) {
	metrics := InitializeMetrics()
	traceEvents := []TraceEvent{}
	jobStartTimes := []JobEvent{}
	jobEndTimes := []JobEvent{}

	metrics.TotalRuns = 1
	if run.Status == "completed" && run.Conclusion == "success" {
		metrics.SuccessfulRuns = 1
	} else {
		metrics.FailedRuns = 1
	}

	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", run.Repository.Owner.Login, run.Repository.Name)
	jobsURL := fmt.Sprintf("%s/actions/runs/%d/jobs?per_page=100", baseURL, run.ID)
	if reporter != nil {
		reporter.SetPhase("Fetching jobs")
		reporter.SetDetail(defaultRunName(run))
	}
	jobs, err := githubapi.FetchJobsPaginated(ctx, client, jobsURL)
	if err != nil {
		return metrics, traceEvents, jobStartTimes, jobEndTimes, err
	}

	runStart, ok := utils.ParseTime(run.CreatedAt)
	if !ok {
		return metrics, traceEvents, jobStartTimes, jobEndTimes, nil
	}
	runEnd, ok := utils.ParseTime(run.UpdatedAt)
	if !ok {
		runEnd = runStart.Add(time.Millisecond)
	}

	workflowURL := fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", run.Repository.Owner.Login, run.Repository.Name, run.ID)

	tid := githubapi.NewTraceID(run.ID, run.RunAttempt)
	sid := githubapi.NewSpanID(run.ID)
	ctx = githubapi.ContextWithIDs(ctx, tid, sid)

	ctx, span := analyzerTracer.Start(ctx, defaultRunName(run),
		trace.WithTimestamp(runStart),
		trace.WithAttributes(
			attribute.String("type", "workflow"),
			attribute.Int64("github.run_id", run.ID),
			attribute.String("github.status", run.Status),
			attribute.String("github.conclusion", run.Conclusion),
			attribute.String("github.repo", fmt.Sprintf("%s/%s", owner, repo)),
			attribute.String("github.url", workflowURL),
		),
	)
	defer span.End(trace.WithTimestamp(runEnd))

	runEndTs := runEnd.UnixMilli()
	runStartTs := runStart.UnixMilli()
	for _, job := range jobs {
		if job.Status != "completed" {
			continue
		}
		if t, ok := utils.ParseTime(job.CompletedAt); ok {
			if t.UnixMilli() > runEndTs {
				runEndTs = t.UnixMilli()
			}
		}
	}
	// Clamp runEndTs to not be too far in the future if the run is completed
	if run.Status == "completed" && runEndTs > runStartTs + 24*3600*1000 {
		// If a run claims to take more than 24h but jobs are short, something is wrong.
		// We'll use the max job end time instead.
		maxJobEnd := runStartTs
		for _, job := range jobs {
			if t, ok := utils.ParseTime(job.CompletedAt); ok {
				if t.UnixMilli() > maxJobEnd {
					maxJobEnd = t.UnixMilli()
				}
			}
		}
		if maxJobEnd > runStartTs {
			runEndTs = maxJobEnd
		}
	}
	runDurationMs := runEndTs - runStartTs
	metrics.TotalDuration += float64(runDurationMs)

	sourceInfo := sourceType
	if sourceType == "pr" {
		sourceInfo = fmt.Sprintf("PR #%s", identifier)
	} else {
		sourceInfo = fmt.Sprintf("commit %s", truncateString(identifier, 8))
	}

	processName := fmt.Sprintf("[%d] %s - %s (%s)", urlIndex+1, sourceInfo, defaultRunName(run), run.Status)
	colors := []string{"#4285f4", "#ea4335", "#fbbc04", "#34a853", "#ff6d01", "#46bdc6", "#7b1fa2", "#d81b60"}
	colorIndex := urlIndex % len(colors)

	traceEvents = append(traceEvents, TraceEvent{
		Name: "process_name",
		Ph:   "M",
		Pid:  processID,
		Args: map[string]interface{}{
			"name":              processName,
			"source_url":        displayURL,
			"source_type":       sourceType,
			"source_identifier": identifier,
			"repository":        fmt.Sprintf("%s/%s", owner, repo),
		},
	})
	traceEvents = append(traceEvents, TraceEvent{
		Name: "process_color",
		Ph:   "M",
		Pid:  processID,
		Args: map[string]interface{}{
			"color":      colors[colorIndex],
			"color_name": fmt.Sprintf("url_%d_color", urlIndex+1),
		},
	})

	workflowThreadID := 1
	AddThreadMetadata(&traceEvents, processID, workflowThreadID, "📋 Workflow Overview", intPtr(0))

	workflowURL = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", run.Repository.Owner.Login, run.Repository.Name, run.ID)
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%s", owner, repo, identifier)

	normalizedRunStart := (runStartTs - earliestTime) * 1000
	normalizedRunEnd := (runEndTs - earliestTime) * 1000
	traceEvents = append(traceEvents, TraceEvent{
		Name: fmt.Sprintf("Workflow: %s [%d]", defaultRunName(run), urlIndex+1),
		Ph:   "X",
		Ts:   normalizedRunStart,
		Dur:  normalizedRunEnd - normalizedRunStart,
		Pid:  processID,
		Tid:  workflowThreadID,
		Cat:  "workflow",
		Args: map[string]interface{}{
			"status":            run.Status,
			"conclusion":        run.Conclusion,
			"run_id":            run.ID,
			"duration_ms":       runDurationMs,
			"job_count":         len(jobs),
			"url":               workflowURL,
			"github_url":        workflowURL,
			"pr_url":            prURL,
			"pr_number":         identifier,
			"repository":        fmt.Sprintf("%s/%s", owner, repo),
			"source_url":        displayURL,
			"source_type":       sourceType,
			"source_identifier": identifier,
			"url_index":         urlIndex + 1,
		},
	})

	for jobIndex, job := range jobs {
		jobThreadID := jobIndex + 10
		processJob(ctx, job, jobIndex, run, jobThreadID, processID, earliestTime, &metrics, &traceEvents, &jobStartTimes, &jobEndTimes, prURL, urlIndex, displayURL, sourceType, identifier)
	}
	return metrics, traceEvents, jobStartTimes, jobEndTimes, nil
}

func processJob(ctx context.Context, job githubapi.Job, jobIndex int, run githubapi.WorkflowRun, jobThreadID, processID int, earliestTime int64, metrics *Metrics, traceEvents *[]TraceEvent, jobStartTimes, jobEndTimes *[]JobEvent, prURL string, urlIndex int, displayURL, sourceType, identifier string) {
	if job.StartedAt == "" {
		return
	}

	jobStart, _ := utils.ParseTime(job.StartedAt)
	jobEnd, _ := utils.ParseTime(job.CompletedAt)

	jobURL := job.HTMLURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d/job/%d", run.Repository.Owner.Login, run.Repository.Name, run.ID, job.ID)
	}

	sid := githubapi.NewSpanID(job.ID)
	ctx = githubapi.ContextWithIDs(ctx, trace.TraceID{}, sid)

	ctx, span := analyzerTracer.Start(ctx, job.Name,
		trace.WithTimestamp(jobStart),
		trace.WithAttributes(
			attribute.String("type", "job"),
			attribute.Int64("github.job_id", job.ID),
			attribute.String("github.status", job.Status),
			attribute.String("github.conclusion", job.Conclusion),
			attribute.String("github.runner_name", job.RunnerName),
			attribute.String("github.url", jobURL),
		),
	)
	defer span.End(trace.WithTimestamp(jobEnd))
	isPending := job.Status != "completed" || job.CompletedAt == ""
	if isPending {
		metrics.PendingJobs = append(metrics.PendingJobs, PendingJob{
			Name:      job.Name,
			Status:    job.Status,
			StartedAt: job.StartedAt,
			URL:       jobURL,
		})
	}

	absoluteJobStart, ok := utils.ParseTime(job.StartedAt)
	if !ok {
		return
	}
	absoluteJobEnd := time.Now()
	if !isPending {
		if t, ok := utils.ParseTime(job.CompletedAt); ok {
			absoluteJobEnd = t
		}
	}

	metrics.TotalJobs++
	if !isPending && (job.Status != "completed" || job.Conclusion != "success") {
		metrics.FailedJobs++
	}

	jobStartTs := absoluteJobStart.UnixMilli()
	jobEndTs := maxInt64(jobStartTs+1, absoluteJobEnd.UnixMilli())
	jobDuration := jobEndTs - jobStartTs
	if jobStartTs >= jobEndTs || jobDuration <= 0 {
		return
	}

	metrics.JobDurations = append(metrics.JobDurations, float64(jobDuration))
	metrics.JobNames = append(metrics.JobNames, job.Name)
	metrics.JobURLs = append(metrics.JobURLs, jobURL)
	if jobDuration > int64(metrics.LongestJob.Duration) {
		metrics.LongestJob = JobDuration{Name: job.Name, Duration: float64(jobDuration)}
	}
	if float64(jobDuration) < metrics.ShortestJob.Duration {
		metrics.ShortestJob = JobDuration{Name: job.Name, Duration: float64(jobDuration)}
	}
	if job.RunnerName != "" {
		metrics.RunnerTypes[job.RunnerName] = struct{}{}
	}

	*jobStartTimes = append(*jobStartTimes, JobEvent{Ts: jobStartTs, Type: "start"})
	*jobEndTimes = append(*jobEndTimes, JobEvent{Ts: jobEndTs, Type: "end"})

	jobIcon := "❓"
	switch {
	case isPending:
		jobIcon = "⏳"
	case job.Conclusion == "success":
		jobIcon = "✅"
	case job.Conclusion == "failure":
		jobIcon = "❌"
	case job.Conclusion == "skipped" || job.Conclusion == "cancelled":
		jobIcon = "⏸️"
	}

	metrics.JobTimeline = append(metrics.JobTimeline, TimelineJob{
		Name:       job.Name,
		StartTime:  jobStartTs,
		EndTime:    jobEndTs,
		Conclusion: job.Conclusion,
		Status:     job.Status,
		URL:        jobURL,
	})

	AddThreadMetadata(traceEvents, processID, jobThreadID, fmt.Sprintf("%s %s", jobIcon, job.Name), intPtr(jobIndex+10))

	normalizedJobStart := (jobStartTs - earliestTime) * 1000
	normalizedJobEnd := (jobEndTs - earliestTime) * 1000
	*traceEvents = append(*traceEvents, TraceEvent{
		Name: fmt.Sprintf("Job: %s [%d]", job.Name, urlIndex+1),
		Ph:   "X",
		Ts:   normalizedJobStart,
		Dur:  normalizedJobEnd - normalizedJobStart,
		Pid:  processID,
		Tid:  jobThreadID,
		Cat:  "job",
		Args: map[string]interface{}{
			"status":            job.Status,
			"conclusion":        job.Conclusion,
			"duration_ms":       jobDuration,
			"runner_name":       defaultString(job.RunnerName, "unknown"),
			"step_count":        len(job.Steps),
			"url":               jobURL,
			"github_url":        jobURL,
			"pr_url":            prURL,
			"pr_number":         lastPathSegment(prURL),
			"repository":        repoFromURL(prURL),
			"job_id":            job.ID,
			"source_url":        displayURL,
			"source_type":       sourceType,
			"source_identifier": identifier,
			"url_index":         urlIndex + 1,
		},
	})

	for _, step := range job.Steps {
		processStep(ctx, step, job, run, jobThreadID, processID, earliestTime, jobEndTs, metrics, traceEvents, prURL, urlIndex, displayURL, sourceType, identifier)
	}
}

func processStep(ctx context.Context, step githubapi.Step, job githubapi.Job, run githubapi.WorkflowRun, jobThreadID, processID int, earliestTime, jobEndTs int64, metrics *Metrics, traceEvents *[]TraceEvent, prURL string, urlIndex int, displayURL, sourceType, identifier string) {
	if step.StartedAt == "" || step.CompletedAt == "" {
		return
	}

	jobURL := job.HTMLURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d/job/%d", run.Repository.Owner.Login, run.Repository.Name, run.ID, job.ID)
	}

	start, ok := utils.ParseTime(step.StartedAt)
	if !ok {
		return
	}
	end, ok := utils.ParseTime(step.CompletedAt)
	if !ok {
		return
	}

	stepURL := fmt.Sprintf("%s#step:%d:1", jobURL, step.Number)

	sid := githubapi.NewSpanIDFromString(fmt.Sprintf("%d-%s", job.ID, step.Name))
	ctx = githubapi.ContextWithIDs(ctx, trace.TraceID{}, sid)

	_, span := analyzerTracer.Start(ctx, step.Name,
		trace.WithTimestamp(start),
		trace.WithAttributes(
			attribute.String("type", "step"),
			attribute.Int("github.step_number", step.Number),
			attribute.String("github.status", step.Status),
			attribute.String("github.conclusion", step.Conclusion),
			attribute.String("github.url", stepURL),
		),
	)
	defer span.End(trace.WithTimestamp(end))

	metrics.TotalSteps++
	if step.Conclusion == "failure" {
		metrics.FailedSteps++
	}

	stepStart := start.UnixMilli()
	stepEnd := maxInt64(stepStart+1, end.UnixMilli())
	if stepEnd > jobEndTs {
		stepEnd = maxInt64(stepStart+1, jobEndTs)
	}
	duration := stepEnd - stepStart
	if stepStart >= stepEnd || duration <= 0 {
		return
	}

	stepIcon := utils.GetStepIcon(step.Name, step.Conclusion)
	stepCategory := utils.CategorizeStep(step.Name)

	metrics.StepDurations = append(metrics.StepDurations, StepDuration{
		Name:     fmt.Sprintf("%s %s", stepIcon, step.Name),
		Duration: float64(duration),
		URL:      stepURL,
		JobName:  job.Name,
	})

	normalizedStepStart := (stepStart - earliestTime) * 1000
	normalizedStepEnd := (stepEnd - earliestTime) * 1000
	*traceEvents = append(*traceEvents, TraceEvent{
		Name: fmt.Sprintf("%s %s [%d]", stepIcon, step.Name, urlIndex+1),
		Ph:   "X",
		Ts:   normalizedStepStart,
		Dur:  normalizedStepEnd - normalizedStepStart,
		Pid:  processID,
		Tid:  jobThreadID,
		Cat:  stepCategory,
		Args: map[string]interface{}{
			"status":            step.Status,
			"conclusion":        step.Conclusion,
			"duration_ms":       duration,
			"job_name":          job.Name,
			"url":               stepURL,
			"github_url":        stepURL,
			"pr_url":            prURL,
			"pr_number":         lastPathSegment(prURL),
			"repository":        repoFromURL(prURL),
			"step_number":       step.Number,
			"source_url":        displayURL,
			"source_type":       sourceType,
			"source_identifier": identifier,
			"url_index":         urlIndex + 1,
		},
	})
}

func addReviewMarkersToTrace(results []URLResult, events *[]TraceEvent) {
	metricsProcessID := 999
	markersThreadID := 2
	AddThreadMetadata(events, metricsProcessID, markersThreadID, "GitHub PR Events", intPtr(1))

	for i := range results {
		result := &results[i]
		if len(result.ReviewEvents) == 0 {
			continue
		}
		timelineStart := result.EarliestTime
		timelineEnd := result.EarliestTime
		if len(result.Metrics.JobTimeline) > 0 {
			timelineStart = result.Metrics.JobTimeline[0].StartTime
			timelineEnd = result.Metrics.JobTimeline[0].EndTime
			for _, job := range result.Metrics.JobTimeline {
				if job.StartTime < timelineStart {
					timelineStart = job.StartTime
				}
				if job.EndTime > timelineEnd {
					timelineEnd = job.EndTime
				}
			}
		}

		for _, event := range result.ReviewEvents {
			originalEventTime := event.TimeMillis()
			ts := (originalEventTime - result.EarliestTime) * 1000
			name := "Approved"
			user := event.Reviewer
			label := utils.YellowText("▲ approved")
			if event.Type == "merged" {
				name = "Merged"
				user = event.MergedBy
				label = utils.GreenText("◆ merged")
			}
			if user != "" {
				if event.Type == "merged" {
					label = utils.GreenText(fmt.Sprintf("◆ merged by %s", user))
				} else {
					label = utils.YellowText(fmt.Sprintf("▲ approved by %s", user))
				}
			}

			userURL := ""
			if user != "" {
				userURL = fmt.Sprintf("https://github.com/%s", user)
			}
			marker := TraceEvent{
				Name: name,
				Ph:   "i",
				S:    "p",
				Ts:   ts,
				Pid:  metricsProcessID,
				Tid:  markersThreadID,
				Args: map[string]interface{}{
					"url_index":              result.URLIndex + 1,
					"source_url":             firstNonEmpty(event.URL, result.DisplayURL),
					"github_url":             firstNonEmpty(event.URL, result.DisplayURL),
					"url":                    firstNonEmpty(event.URL, result.DisplayURL),
					"source_type":            result.Type,
					"source_identifier":      result.Identifier,
					"user":                   user,
					"user_url":               userURL,
					"label":                  label,
					"original_event_time_ms": originalEventTime,
					"clamped":                false,
				},
			}
			*events = append(*events, marker)
			// Also add to the individual result so ingestors see it
			result.TraceEvents = append(result.TraceEvents, marker)
		}
	}
}

func shipItMatch(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "ship it") || strings.Contains(lower, "shipit")
}

func resolvedUser(user *githubapi.UserInfo) string {
	if user == nil {
		return ""
	}
	if user.Login != "" {
		return user.Login
	}
	return user.Name
}

func truncateString(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func defaultRunName(run githubapi.WorkflowRun) string {
	if run.Name != "" {
		return run.Name
	}
	return fmt.Sprintf("Run %d", run.ID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func lastPathSegment(urlValue string) string {
	parts := strings.Split(strings.TrimRight(urlValue, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func repoFromURL(urlValue string) string {
	parts := strings.Split(strings.TrimRight(urlValue, "/"), "/")
	if len(parts) < 4 {
		return ""
	}
	return strings.Join(parts[len(parts)-4:len(parts)-2], "/")
}

func maxJobEnd(events []JobEvent) int64 {
	max := int64(0)
	for _, event := range events {
		if event.Type == "end" && event.Ts > max {
			max = event.Ts
		}
	}
	return max
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func mergeMetrics(target *Metrics, source Metrics) {
	target.TotalRuns += source.TotalRuns
	target.SuccessfulRuns += source.SuccessfulRuns
	target.FailedRuns += source.FailedRuns
	target.TotalJobs += source.TotalJobs
	target.FailedJobs += source.FailedJobs
	target.TotalSteps += source.TotalSteps
	target.FailedSteps += source.FailedSteps
	target.TotalDuration += source.TotalDuration

	target.JobDurations = append(target.JobDurations, source.JobDurations...)
	target.JobNames = append(target.JobNames, source.JobNames...)
	target.JobURLs = append(target.JobURLs, source.JobURLs...)
	target.StepDurations = append(target.StepDurations, source.StepDurations...)
	target.JobTimeline = append(target.JobTimeline, source.JobTimeline...)
	target.PendingJobs = append(target.PendingJobs, source.PendingJobs...)

	for runner := range source.RunnerTypes {
		target.RunnerTypes[runner] = struct{}{}
	}

	if source.LongestJob.Duration > target.LongestJob.Duration {
		target.LongestJob = source.LongestJob
	}
	if source.ShortestJob.Duration < target.ShortestJob.Duration {
		target.ShortestJob = source.ShortestJob
	}
}
