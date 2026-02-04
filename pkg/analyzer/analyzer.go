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
	Window time.Duration
}

func AnalyzeURLs(ctx context.Context, urls []string, client githubapi.GitHubProvider, reporter ProgressReporter, opts AnalyzeOptions) ([]URLResult, []TraceEvent, int64, int64, []URLError) {
	allTraceEvents := []TraceEvent{}
	allJobStartTimes := []JobEvent{}
	allJobEndTimes := []JobEvent{}
	urlResults := []URLResult{}
	globalEarliestTime := int64(1<<63 - 1)
	globalLatestTime := int64(0)
	urlErrors := []URLError{}

	provider := NewDataProvider(client)
	emitter := NewTraceEmitter(analyzerTracer)

	for urlIndex, githubURL := range urls {
		if reporter != nil {
			reporter.StartURL(urlIndex, githubURL)
		}
		
		rawData, err := provider.Fetch(ctx, githubURL, urlIndex, reporter, opts)
		if err != nil {
			urlErrors = append(urlErrors, URLError{URL: githubURL, Err: err})
			continue
		}
		if rawData == nil {
			continue
		}

		// Emit markers before processing runs to ensure chronological order in collector
		emitter.EmitMarkers(ctx, rawData)

		// Calculate urlEarliestTime here to ensure it's consistent
		urlEarliestTime := FindEarliestTimestamp(rawData.Runs)
		if rawData.CommitTimeMs != nil && *rawData.CommitTimeMs < urlEarliestTime {
			urlEarliestTime = *rawData.CommitTimeMs
		}
		if rawData.CommitPushedAtMs != nil && *rawData.CommitPushedAtMs < urlEarliestTime {
			urlEarliestTime = *rawData.CommitPushedAtMs
		}
		for _, event := range rawData.ReviewEvents {
			ms := event.TimeMillis()
			if ms < urlEarliestTime {
				urlEarliestTime = ms
			}
		}

		result, err := buildURLResult(ctx, rawData.Parsed, urlIndex, rawData.HeadSHA, rawData.BranchName, rawData.DisplayName, rawData.DisplayURL, rawData.ReviewEvents, rawData.MergedAtMs, rawData.CommitTimeMs, rawData.CommitPushedAtMs, rawData.AllCommitRunsCount, rawData.AllCommitRunsComputeMs, rawData.Runs, rawData.RequiredContexts, client, reporter, urlEarliestTime)
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

func buildURLResult(ctx context.Context, parsed utils.ParsedGitHubURL, urlIndex int, headSHA, branchName, displayName, displayURL string, reviewEvents []ReviewEvent, mergedAtMs, commitTimeMs, commitPushedAtMs *int64, allCommitRunsCount int, allCommitRunsComputeMs int64, runs []githubapi.WorkflowRun, requiredContexts []string, client githubapi.GitHubProvider, reporter ProgressReporter, urlEarliestTime int64) (*URLResult, error) {
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
				runMetrics, runTrace, runStarts, runEnds, err := processWorkflowRun(ctx, job.run, job.index, processID, urlEarliestTime, parsed.Owner, parsed.Repo, parsed.Identifier, urlIndex, displayURL, parsed.Type, requiredContexts, client, reporter)
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
		CommitPushedAtMs:       commitPushedAtMs,
		AllCommitRunsCount:     allCommitRunsCount,
		AllCommitRunsComputeMs: allCommitRunsComputeMs,
	}
	return &result, nil
}

func processWorkflowRun(ctx context.Context, run githubapi.WorkflowRun, runIndex, processID int, earliestTime int64, owner, repo, identifier string, urlIndex int, displayURL, sourceType string, requiredContexts []string, client githubapi.GitHubProvider, reporter ProgressReporter) (Metrics, []TraceEvent, []JobEvent, []JobEvent, error) {
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
	jobs, err := client.FetchJobsPaginated(ctx, jobsURL)
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
	if run.Status == "completed" && runEndTs > runStartTs+24*3600*1000 {
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
		processJob(ctx, job, jobIndex, run, jobThreadID, processID, earliestTime, &metrics, &traceEvents, &jobStartTimes, &jobEndTimes, prURL, urlIndex, displayURL, sourceType, identifier, requiredContexts)
	}
	return metrics, traceEvents, jobStartTimes, jobEndTimes, nil
}

func processJob(ctx context.Context, job githubapi.Job, jobIndex int, run githubapi.WorkflowRun, jobThreadID, processID int, earliestTime int64, metrics *Metrics, traceEvents *[]TraceEvent, jobStartTimes, jobEndTimes *[]JobEvent, prURL string, urlIndex int, displayURL, sourceType, identifier string, requiredContexts []string) {
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

	// Determine if this job is a required status check
	isRequired := isJobRequired(job.Name, run.Name, requiredContexts)
	requiredSuffix := ""
	if isRequired {
		requiredSuffix = " 🔒"
	}

	ctx, span := analyzerTracer.Start(ctx, job.Name+requiredSuffix,
		trace.WithTimestamp(jobStart),
		trace.WithAttributes(
			attribute.String("type", "job"),
			attribute.Int64("github.job_id", job.ID),
			attribute.String("github.status", job.Status),
			attribute.String("github.conclusion", job.Conclusion),
			attribute.String("github.runner_name", job.RunnerName),
			attribute.String("github.url", jobURL),
			attribute.Bool("is_required", isRequired),
		),
	)
	defer span.End(trace.WithTimestamp(jobEnd))

	isPending := job.Status != "completed" || job.CompletedAt == ""
	if isPending {
		metrics.PendingJobs = append(metrics.PendingJobs, PendingJob{
			Name:       job.Name,
			Status:     job.Status,
			StartedAt:  job.StartedAt,
			URL:        jobURL,
			IsRequired: isRequired,
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
	if isPending {
		jobIcon = "⏳"
	} else if job.Conclusion == "failure" {
		// No icon here, handled in output rendering
	} else if job.Conclusion == "skipped" || job.Conclusion == "cancelled" {
		jobIcon = "⏸️"
	}

	metrics.JobTimeline = append(metrics.JobTimeline, TimelineJob{
		Name:       job.Name,
		StartTime:  jobStartTs,
		EndTime:    jobEndTs,
		Conclusion: job.Conclusion,
		Status:     job.Status,
		URL:        jobURL,
		IsRequired: isRequired,
	})

	jobLabel := fmt.Sprintf("%s %s%s", jobIcon, job.Name, requiredSuffix)
	if job.Conclusion == "failure" {
		jobLabel = fmt.Sprintf("%s%s ❌", job.Name, requiredSuffix)
	}
	AddThreadMetadata(traceEvents, processID, jobThreadID, jobLabel, intPtr(jobIndex+10))

	normalizedJobStart := (jobStartTs - earliestTime) * 1000
	normalizedJobEnd := (jobEndTs - earliestTime) * 1000
	*traceEvents = append(*traceEvents, TraceEvent{
		Name: fmt.Sprintf("Job: %s%s [%d]", job.Name, requiredSuffix, urlIndex+1),
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
			"is_required":       isRequired,
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

func addReviewMarkersToTrace(urlResults []URLResult, events *[]TraceEvent) {
	metricsProcessID := 999
	markersThreadID := 2
	AddThreadMetadata(events, metricsProcessID, markersThreadID, "GitHub PR Events", intPtr(1))

	for i := range urlResults {
		result := &urlResults[i]
		if len(result.ReviewEvents) == 0 {
			continue
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

func isJobRequired(jobName, workflowName string, requiredContexts []string) bool {
	if len(requiredContexts) == 0 {
		return true // No branch protection = treat all as required (preserve current behavior)
	}

	// GitHub status checks can match: "workflow / job", "job", or "workflow"
	fullName := fmt.Sprintf("%s / %s", workflowName, jobName)

	for _, ctx := range requiredContexts {
		if ctx == fullName || ctx == jobName || ctx == workflowName {
			return true
		}
		// Handle matrix jobs: "test (ubuntu, 18)" matches "test"
		if strings.HasPrefix(fullName, ctx) || strings.HasPrefix(jobName, ctx) {
			return true
		}
	}
	return false
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
