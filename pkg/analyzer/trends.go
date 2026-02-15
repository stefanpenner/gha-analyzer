package analyzer

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
)

// SamplingInfo describes whether and how sampling was applied
type SamplingInfo struct {
	Enabled        bool
	SampleSize     int     // job-level sample count
	TotalRuns      int     // total runs fetched from API
	Confidence     float64
	MarginOfError  float64
	Rationale      string  // human-readable "why we think" explanation
}

// TrendAnalysis holds the result of analyzing historical workflow trends for a repository.
type TrendAnalysis struct {
	Owner            string
	Repo             string
	TimeRange        TimeRange
	Sampling         SamplingInfo
	Summary          TrendSummary
	DurationTrend    []DataPoint
	SuccessRateTrend []DataPoint
	JobTrends        []JobTrend
	FlakyJobs        []FlakyJob
	TopRegressions   []JobRegression
	TopImprovements  []JobImprovement
	QueueTimeStats   QueueTimeStats
}

// Changepoint identifies the approximate point in time where a job's duration shifted.
type Changepoint struct {
	Date         time.Time // timestamp of the first observation after the shift
	BeforeSHA    string    // last commit SHA before the changepoint
	AfterSHA     string    // first commit SHA after the changepoint
	BeforeRunURL string    // URL of the last run before the changepoint
	AfterRunURL  string    // URL of the first run after the changepoint
	DiffURL      string    // GitHub compare URL: BeforeSHA...AfterSHA
	BeforeAvg    float64   // average duration (seconds) before the changepoint
	AfterAvg     float64   // average duration (seconds) after the changepoint
	Index        int       // index of the first observation after the shift
	TotalPoints  int       // total number of observations
}

// JobRegression represents a job that got slower
type JobRegression struct {
	Name            string
	URLs            []string // sample recent job URLs (newest first)
	OldAvgDuration  float64
	NewAvgDuration  float64
	PercentIncrease float64
	AbsoluteChange  float64
	Changepoint     *Changepoint // nil when insufficient data
}

// JobImprovement represents a job that got faster
type JobImprovement struct {
	Name            string
	URLs            []string // sample recent job URLs (newest first)
	OldAvgDuration  float64
	NewAvgDuration  float64
	PercentDecrease float64
	AbsoluteChange  float64
	Changepoint     *Changepoint // nil when insufficient data
}

// QueueTimeStats contains queue time analysis
type QueueTimeStats struct {
	AvgQueueTime    float64
	MedianQueueTime float64
	P95QueueTime    float64
	AvgRunTime      float64
	MedianRunTime   float64
	QueueTimeRatio  float64 // queue time / total time
}

// TimeRange represents a time period
type TimeRange struct {
	Start time.Time
	End   time.Time
	Days  int
}

// TrendSummary provides high-level statistics
type TrendSummary struct {
	TotalRuns          int
	AvgDuration        float64
	MedianDuration     float64
	P95Duration        float64
	AvgSuccessRate     float64
	TrendDirection     string // "improving", "stable", "degrading"
	TrendDescription   string // human-readable explanation of the trend
	PercentChange      float64
	MostFlakyJobsCount int
}

// DataPoint represents a single data point in a trend
type DataPoint struct {
	Timestamp time.Time
	Value     float64
	Count     int // number of runs at this point
}

// JobTrend contains trend data for a specific job
type JobTrend struct {
	Name           string
	URLs           []string // sample recent job URLs (newest first)
	AvgDuration    float64
	MedianDuration float64
	SuccessRate    float64
	TotalRuns      int
	TrendDirection string
	DurationPoints []DataPoint
}

// FlakyJob represents a job with inconsistent outcomes
type FlakyJob struct {
	Name           string
	URLs           []string // sample recent failure URLs (newest first)
	TotalRuns      int
	SuccessCount   int
	FailureCount   int
	FlakeRate      float64 // percentage of failures
	RecentFailures int     // failures in last 10 runs
	LastFailure    time.Time
}

// RunData represents simplified workflow run data
type RunData struct {
	ID         int64
	HeadSHA    string
	Status     string
	Conclusion string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Duration   int64 // milliseconds
	Jobs       []JobData
}

// JobData represents simplified job data
type JobData struct {
	ID          int64
	Name        string
	URL         string
	Status      string
	Conclusion  string
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    int64 // milliseconds
	QueueTime   int64 // milliseconds
}

// TrendOptions configures the trend analysis behavior
type TrendOptions struct {
	NoSample    bool
	Confidence  float64 // e.g. 0.95 for 95%
	MarginOfError float64 // e.g. 0.10 for ±10%
}

// AnalyzeTrends analyzes historical trends for a repository using GitHub API.
// All run pages are always fetched to ensure accurate trend detection.
// When opts.NoSample is false (default), job detail fetching uses statistical
// sampling to reduce API calls. Run-level metrics use all fetched runs.
func AnalyzeTrends(ctx context.Context, client githubapi.GitHubProvider, owner, repo string, days int, branch, workflow string, opts TrendOptions, reporter ProgressReporter) (*TrendAnalysis, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(days) * 24 * time.Hour)

	confidence := opts.Confidence
	if confidence <= 0 {
		confidence = 0.95
	}
	marginOfError := opts.MarginOfError
	if marginOfError <= 0 {
		marginOfError = 0.10
	}

	fetchDetail := fmt.Sprintf("%s/%s, last %d days", owner, repo, days)
	if branch != "" {
		fetchDetail += fmt.Sprintf(", branch: %s", branch)
	}
	if workflow != "" {
		fetchDetail += fmt.Sprintf(", workflow: %s", workflow)
	}

	if reporter != nil {
		reporter.SetPhase("Fetching workflow runs")
		reporter.SetDetail(fetchDetail)
	}

	sampling := SamplingInfo{
		Confidence:    confidence,
		MarginOfError: marginOfError,
	}

	// Fetch all run pages — run listing is cheap (1 API call per 100 runs)
	// and complete data is needed for accurate trend detection.
	var onPage func(fetched, total int)
	if reporter != nil {
		onPage = func(fetched, total int) {
			if total > 0 {
				reporter.SetDetail(fmt.Sprintf("%s — %d/%d runs", fetchDetail, fetched, total))
			} else {
				reporter.SetDetail(fmt.Sprintf("%s — %d runs", fetchDetail, fetched))
			}
		}
	}
	runs, err := client.FetchRecentWorkflowRuns(ctx, owner, repo, days, branch, workflow, onPage)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no workflow runs found for %s/%s in the last %d days", owner, repo, days)
	}

	// Convert all runs to RunData (no job fetching yet)
	runData := convertRuns(runs)

	// Determine job-level sampling.
	// The user's margin of error applies to population-level estimates, but
	// per-job trend detection splits observations across many distinct jobs.
	// Use a tighter effective margin (÷3) so that even medium-frequency jobs
	// retain enough observations per half for reliable trend detection.
	totalRuns := len(runData)
	sampling.TotalRuns = totalRuns
	jobMargin := marginOfError / 3
	sampleSize := calculateSampleSize(totalRuns, confidence, jobMargin)

	// Only sample when it saves ≥25% of API calls; otherwise the marginal
	// savings aren't worth the per-job accuracy loss on borderline trends.
	if !opts.NoSample && sampleSize < totalRuns*3/4 {
		sampling.Enabled = true
		sampling.SampleSize = sampleSize
	} else {
		sampling.SampleSize = totalRuns
	}

	// Generate rationale
	sampling.Rationale = generateRationale(sampling)

	if reporter != nil {
		reporter.SetURLRuns(sampling.SampleSize)
		if sampling.Enabled {
			reporter.SetPhase("Fetching job details")
			reporter.SetDetail(fmt.Sprintf("sampling %d/%d runs — %.0f%% confidence, ±%.0f%% margin",
				sampling.SampleSize, sampling.TotalRuns,
				sampling.Confidence*100, sampling.MarginOfError*100))
		} else {
			reporter.SetPhase("Fetching job details")
			reporter.SetDetail(fmt.Sprintf("%d runs", totalRuns))
		}
	}

	// Fetch jobs for sampled runs
	sampleIndices := sampleRunIndices(runs, sampling.SampleSize)
	if err := fetchJobsForRuns(ctx, client, runData, runs, sampleIndices, reporter); err != nil {
		return nil, fmt.Errorf("failed to fetch job data: %w", err)
	}

	if reporter != nil {
		reporter.SetPhase("Analyzing trends")
	}

	// Sort runs chronologically (oldest first) so first-half/second-half
	// comparisons correctly treat early data as "before" and recent data as "after".
	// The GitHub API returns runs newest-first by default.
	sort.Slice(runData, func(i, j int) bool {
		return runData[i].CreatedAt.Before(runData[j].CreatedAt)
	})

	analysis := &TrendAnalysis{
		Owner: owner,
		Repo:  repo,
		TimeRange: TimeRange{
			Start: startTime,
			End:   endTime,
			Days:  days,
		},
		Sampling: sampling,
	}

	// Calculate summary statistics (uses all runs — run-level data)
	analysis.Summary = calculateTrendSummary(runData)

	// Generate duration trend (uses all runs)
	analysis.DurationTrend = generateDurationTrend(runData)

	// Generate success rate trend (uses all runs)
	analysis.SuccessRateTrend = generateSuccessRateTrend(runData)

	// Analyze individual jobs (uses sampled job data)
	analysis.JobTrends = analyzeJobTrends(runData)

	// Detect flaky jobs (uses sampled job data)
	analysis.FlakyJobs = detectFlakyJobs(runData)
	analysis.Summary.MostFlakyJobsCount = len(analysis.FlakyJobs)

	// Calculate regressions and improvements (uses sampled job data)
	analysis.TopRegressions, analysis.TopImprovements = calculateJobChanges(runData)

	// Populate diff URLs on changepoints
	for i, reg := range analysis.TopRegressions {
		if reg.Changepoint != nil {
			analysis.TopRegressions[i].Changepoint.DiffURL = fmt.Sprintf(
				"https://github.com/%s/%s/compare/%s...%s", owner, repo, reg.Changepoint.BeforeSHA, reg.Changepoint.AfterSHA)
		}
	}
	for i, imp := range analysis.TopImprovements {
		if imp.Changepoint != nil {
			analysis.TopImprovements[i].Changepoint.DiffURL = fmt.Sprintf(
				"https://github.com/%s/%s/compare/%s...%s", owner, repo, imp.Changepoint.BeforeSHA, imp.Changepoint.AfterSHA)
		}
	}

	// Calculate queue time statistics (uses sampled job data)
	analysis.QueueTimeStats = calculateQueueTimeStats(runData)

	return analysis, nil
}

// calculateSampleSize computes the minimum sample size for a finite population
// using the standard formula: n = n₀ / (1 + (n₀-1)/N)
// where n₀ = Z² × p × (1-p) / E²
func calculateSampleSize(totalRuns int, confidence, marginOfError float64) int {
	if totalRuns <= 0 {
		return 0
	}

	// Z-score lookup for common confidence levels
	z := 1.96 // default 95%
	switch {
	case confidence >= 0.99:
		z = 2.576
	case confidence >= 0.98:
		z = 2.326
	case confidence >= 0.95:
		z = 1.96
	case confidence >= 0.90:
		z = 1.645
	}

	p := 0.5 // maximum variance
	n0 := (z * z * p * (1 - p)) / (marginOfError * marginOfError)
	n := n0 / (1 + (n0-1)/float64(totalRuns))

	size := int(math.Ceil(n))
	if size > totalRuns {
		size = totalRuns
	}
	return size
}

// sampleRunIndices returns indices of runs to fetch jobs for.
// Uses a deterministic seed derived from run IDs for reproducibility.
func sampleRunIndices(runs []githubapi.WorkflowRun, sampleSize int) []int {
	total := len(runs)
	if sampleSize >= total {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	// Deterministic seed from run IDs
	h := fnv.New64a()
	for _, run := range runs {
		fmt.Fprintf(h, "%d", run.ID)
	}
	rng := rand.New(rand.NewSource(int64(h.Sum64())))

	// Fisher-Yates partial shuffle to select sampleSize unique indices
	indices := make([]int, total)
	for i := range indices {
		indices[i] = i
	}
	for i := 0; i < sampleSize; i++ {
		j := i + rng.Intn(total-i)
		indices[i], indices[j] = indices[j], indices[i]
	}
	selected := indices[:sampleSize]
	sort.Ints(selected)
	return selected
}

// generateRationale produces a human-readable explanation of sampling decisions.
func generateRationale(s SamplingInfo) string {
	parts := []string{fmt.Sprintf("%s runs analyzed.", formatCount(s.TotalRuns))}

	if s.Enabled {
		parts = append(parts, fmt.Sprintf("%d sampled for job details (%.0f%% confidence, ±%.0f%% margin).",
			s.SampleSize, s.Confidence*100, s.MarginOfError*100))
	} else {
		parts = append(parts, "Full job details fetched for all runs.")
	}

	return strings.Join(parts, " ")
}

// formatCount formats a number with commas for readability.
func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// convertRuns converts WorkflowRun data to RunData without fetching jobs
func convertRuns(runs []githubapi.WorkflowRun) []RunData {
	runData := make([]RunData, len(runs))
	for i, run := range runs {
		createdAt, _ := utils.ParseTime(run.CreatedAt)
		updatedAt, _ := utils.ParseTime(run.UpdatedAt)
		rd := RunData{
			ID:         run.ID,
			HeadSHA:    run.HeadSHA,
			Status:     run.Status,
			Conclusion: run.Conclusion,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		}
		if !createdAt.IsZero() && !updatedAt.IsZero() {
			rd.Duration = updatedAt.Sub(createdAt).Milliseconds()
		}
		runData[i] = rd
	}
	return runData
}

// fetchJobsForRuns fetches job details for runs at the given indices
func fetchJobsForRuns(ctx context.Context, client githubapi.GitHubProvider, runData []RunData, runs []githubapi.WorkflowRun, indices []int, reporter ProgressReporter) error {
	for _, idx := range indices {
		run := runs[idx]
		jobsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/jobs",
			run.Repository.Owner.Login, run.Repository.Name, run.ID)
		jobs, err := client.FetchJobsPaginated(ctx, jobsURL)
		if reporter != nil {
			reporter.ProcessRun()
		}
		if err != nil {
			continue
		}
		for _, job := range jobs {
			createdAt, _ := utils.ParseTime(job.CreatedAt)
			startedAt, _ := utils.ParseTime(job.StartedAt)
			completedAt, _ := utils.ParseTime(job.CompletedAt)

			duration := int64(0)
			if !startedAt.IsZero() && !completedAt.IsZero() {
				duration = completedAt.Sub(startedAt).Milliseconds()
			}

			queueTime := int64(0)
			if !createdAt.IsZero() && !startedAt.IsZero() {
				queueTime = startedAt.Sub(createdAt).Milliseconds()
			}

			runData[idx].Jobs = append(runData[idx].Jobs, JobData{
				ID:          job.ID,
				Name:        job.Name,
				URL:         job.HTMLURL,
				Status:      job.Status,
				Conclusion:  job.Conclusion,
				CreatedAt:   createdAt,
				StartedAt:   startedAt,
				CompletedAt: completedAt,
				Duration:    duration,
				QueueTime:   queueTime,
			})
		}
	}
	return nil
}

// calculateTrendSummary computes summary statistics
func calculateTrendSummary(runs []RunData) TrendSummary {
	if len(runs) == 0 {
		return TrendSummary{}
	}

	durations := make([]float64, 0, len(runs))
	successCount := 0

	for _, run := range runs {
		if run.Duration > 0 {
			durationSec := float64(run.Duration) / 1000.0
			durations = append(durations, durationSec)
		}
		if run.Conclusion == "success" {
			successCount++
		}
	}

	avgDuration := average(durations)
	medianDuration := calculateMedian(durations)
	p95Duration := calculatePercentile(durations, 95)
	avgSuccessRate := float64(successCount) / float64(len(runs)) * 100

	// Determine trend direction
	trendDirection := "stable"
	percentChange := 0.0
	if len(durations) >= 4 {
		midpoint := len(durations) / 2
		firstHalf := average(durations[:midpoint])
		secondHalf := average(durations[midpoint:])

		if firstHalf > 0 {
			percentChange = ((secondHalf - firstHalf) / firstHalf) * 100
			if percentChange < -5 {
				trendDirection = "improving"
			} else if percentChange > 5 {
				trendDirection = "degrading"
			}
		}
	}

	// Generate human-readable trend description
	trendDescription := ""
	switch trendDirection {
	case "improving":
		trendDescription = fmt.Sprintf("Workflow durations decreased by %.1f%% over this period. Runs are completing faster in the more recent half of the analysis window.", -percentChange)
	case "degrading":
		trendDescription = fmt.Sprintf("Workflow durations increased by %.1f%% over this period. Runs are taking longer in the more recent half of the analysis window. Investigate recent changes for added overhead, cache issues, or resource contention.", percentChange)
	case "stable":
		trendDescription = "Workflow durations are stable (within 5% variation) over this period."
	}

	return TrendSummary{
		TotalRuns:        len(runs),
		AvgDuration:      avgDuration,
		MedianDuration:   medianDuration,
		P95Duration:      p95Duration,
		AvgSuccessRate:   avgSuccessRate,
		TrendDirection:   trendDirection,
		TrendDescription: trendDescription,
		PercentChange:    percentChange,
	}
}

// generateDurationTrend creates time-series data for duration
func generateDurationTrend(runs []RunData) []DataPoint {
	dayBuckets := make(map[string][]float64)

	for _, run := range runs {
		if run.Duration > 0 {
			dayKey := run.CreatedAt.Format("2006-01-02")
			durationSec := float64(run.Duration) / 1000.0
			dayBuckets[dayKey] = append(dayBuckets[dayKey], durationSec)
		}
	}

	var points []DataPoint
	for dayKey, durations := range dayBuckets {
		timestamp, _ := time.Parse("2006-01-02", dayKey)
		avgDuration := average(durations)
		points = append(points, DataPoint{
			Timestamp: timestamp,
			Value:     avgDuration,
			Count:     len(durations),
		})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points
}

// generateSuccessRateTrend creates time-series data for success rate
func generateSuccessRateTrend(runs []RunData) []DataPoint {
	dayBuckets := make(map[string]struct{ success, total int })

	for _, run := range runs {
		dayKey := run.CreatedAt.Format("2006-01-02")
		bucket := dayBuckets[dayKey]
		bucket.total++
		if run.Conclusion == "success" {
			bucket.success++
		}
		dayBuckets[dayKey] = bucket
	}

	var points []DataPoint
	for dayKey, bucket := range dayBuckets {
		timestamp, _ := time.Parse("2006-01-02", dayKey)
		successRate := float64(bucket.success) / float64(bucket.total) * 100
		points = append(points, DataPoint{
			Timestamp: timestamp,
			Value:     successRate,
			Count:     bucket.total,
		})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points
}

// analyzeJobTrends analyzes individual job trends
func analyzeJobTrends(runs []RunData) []JobTrend {
	// Collect all jobs by name
	jobMap := make(map[string][]JobData)

	for _, run := range runs {
		for _, job := range run.Jobs {
			jobMap[job.Name] = append(jobMap[job.Name], job)
		}
	}

	var trends []JobTrend

	for name, jobs := range jobMap {
		durations := make([]float64, 0, len(jobs))
		successCount := 0

		for _, job := range jobs {
			if job.Duration > 0 {
				durationSec := float64(job.Duration) / 1000.0
				durations = append(durations, durationSec)
			}
			if job.Conclusion == "success" {
				successCount++
			}
		}

		avgDuration := average(durations)
		medianDuration := calculateMedian(durations)
		successRate := float64(successCount) / float64(len(jobs)) * 100

		// Determine trend direction
		trendDirection := "stable"
		if len(durations) >= 4 {
			midpoint := len(durations) / 2
			firstHalf := average(durations[:midpoint])
			secondHalf := average(durations[midpoint:])

			if firstHalf > 0 {
				percentChange := ((secondHalf - firstHalf) / firstHalf) * 100
				if percentChange < -5 {
					trendDirection = "improving"
				} else if percentChange > 5 {
					trendDirection = "degrading"
				}
			}
		}

		// Collect up to 5 most recent URLs (jobs are oldest-first)
		var urls []string
		for i := len(jobs) - 1; i >= 0 && len(urls) < 5; i-- {
			if jobs[i].URL != "" {
				urls = append(urls, jobs[i].URL)
			}
		}

		trends = append(trends, JobTrend{
			Name:           name,
			URLs:           urls,
			AvgDuration:    avgDuration,
			MedianDuration: medianDuration,
			SuccessRate:    successRate,
			TotalRuns:      len(jobs),
			TrendDirection: trendDirection,
		})
	}

	// Sort by average duration (slowest first)
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].AvgDuration > trends[j].AvgDuration
	})

	return trends
}

// detectFlakyJobs identifies jobs with inconsistent outcomes
func detectFlakyJobs(runs []RunData) []FlakyJob {
	jobMap := make(map[string][]JobData)

	for _, run := range runs {
		for _, job := range run.Jobs {
			jobMap[job.Name] = append(jobMap[job.Name], job)
		}
	}

	var flakyJobs []FlakyJob

	for name, jobs := range jobMap {
		if len(jobs) < 5 {
			continue // Not enough data
		}

		successCount := 0
		failureCount := 0
		var lastFailure time.Time
		var failureURLs []string

		// Sort jobs by time (most recent first)
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].CompletedAt.After(jobs[j].CompletedAt)
		})

		recentFailures := 0
		recentLimit := 10
		if len(jobs) < recentLimit {
			recentLimit = len(jobs)
		}

		for i, job := range jobs {
			if job.Conclusion == "success" {
				successCount++
			} else if job.Conclusion == "failure" {
				failureCount++
				if lastFailure.IsZero() || job.CompletedAt.After(lastFailure) {
					lastFailure = job.CompletedAt
				}
				if job.URL != "" && len(failureURLs) < 5 {
					failureURLs = append(failureURLs, job.URL)
				}

				if i < recentLimit {
					recentFailures++
				}
			}
		}

		if failureCount == 0 {
			continue // Never failed, not flaky
		}

		flakeRate := float64(failureCount) / float64(len(jobs)) * 100

		// Only include if flake rate > 10%
		if flakeRate > 10 {
			flakyJobs = append(flakyJobs, FlakyJob{
				Name:           name,
				URLs:           failureURLs,
				TotalRuns:      len(jobs),
				SuccessCount:   successCount,
				FailureCount:   failureCount,
				FlakeRate:      flakeRate,
				RecentFailures: recentFailures,
				LastFailure:    lastFailure,
			})
		}
	}

	// Sort by flake rate (worst first)
	sort.Slice(flakyJobs, func(i, j int) bool {
		return flakyJobs[i].FlakeRate > flakyJobs[j].FlakeRate
	})

	return flakyJobs
}

// Utility functions

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func calculatePercentile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := int(math.Ceil(float64(len(sorted)) * float64(p) / 100.0)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// jobObservation captures per-observation metadata for changepoint detection.
type jobObservation struct {
	DurationSec  float64
	RunCreatedAt time.Time
	HeadSHA      string
	JobURL       string
}

// detectChangepoint finds the single split point that minimizes total
// sum-of-squared residuals. Returns nil if len(observations) < 2*minSideSize.
func detectChangepoint(observations []jobObservation, minSideSize int) *Changepoint {
	n := len(observations)
	if n < 2*minSideSize {
		return nil
	}

	bestSSR := math.MaxFloat64
	bestIdx := -1

	for i := minSideSize; i <= n-minSideSize; i++ {
		leftAvg := avgObservations(observations[:i])
		rightAvg := avgObservations(observations[i:])

		ssr := 0.0
		for j := 0; j < i; j++ {
			d := observations[j].DurationSec - leftAvg
			ssr += d * d
		}
		for j := i; j < n; j++ {
			d := observations[j].DurationSec - rightAvg
			ssr += d * d
		}
		if ssr < bestSSR {
			bestSSR = ssr
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return nil
	}

	beforeAvg := avgObservations(observations[:bestIdx])
	afterAvg := avgObservations(observations[bestIdx:])
	last := observations[bestIdx-1]
	first := observations[bestIdx]

	return &Changepoint{
		Date:         first.RunCreatedAt,
		BeforeSHA:    last.HeadSHA,
		AfterSHA:     first.HeadSHA,
		BeforeRunURL: last.JobURL,
		AfterRunURL:  first.JobURL,
		BeforeAvg:    beforeAvg,
		AfterAvg:     afterAvg,
		Index:        bestIdx,
		TotalPoints:  n,
	}
}

// avgObservations computes the mean DurationSec of a slice of observations.
func avgObservations(obs []jobObservation) float64 {
	if len(obs) == 0 {
		return 0
	}
	sum := 0.0
	for _, o := range obs {
		sum += o.DurationSec
	}
	return sum / float64(len(obs))
}

// calculateJobChanges finds jobs that got significantly slower or faster
func calculateJobChanges(runs []RunData) ([]JobRegression, []JobImprovement) {
	if len(runs) < 4 {
		return nil, nil // Not enough data
	}

	// Group jobs by name, preserving per-observation metadata (chronological order)
	jobMap := make(map[string][]jobObservation)

	for _, run := range runs {
		for _, job := range run.Jobs {
			if job.Duration <= 0 {
				continue
			}
			jobMap[job.Name] = append(jobMap[job.Name], jobObservation{
				DurationSec:  float64(job.Duration) / 1000.0,
				RunCreatedAt: run.CreatedAt,
				HeadSHA:      run.HeadSHA,
				JobURL:       job.URL,
			})
		}
	}

	var regressions []JobRegression
	var improvements []JobImprovement

	for name, observations := range jobMap {
		midpoint := len(observations) / 2
		firstHalf := observations[:midpoint]
		secondHalf := observations[midpoint:]

		if len(firstHalf) < 2 || len(secondHalf) < 2 {
			continue
		}

		oldAvg := avgObservations(firstHalf)
		newAvg := avgObservations(secondHalf)
		change := newAvg - oldAvg
		percentChange := (change / oldAvg) * 100

		// Only include significant changes (>10%)
		if math.Abs(percentChange) < 10 {
			continue
		}

		// Take up to 5 most recent URLs (newest first)
		var urls []string
		for i := len(secondHalf) - 1; i >= 0 && len(urls) < 5; i-- {
			if secondHalf[i].JobURL != "" {
				urls = append(urls, secondHalf[i].JobURL)
			}
		}

		cp := detectChangepoint(observations, 3)

		if change > 0 {
			// Regression (got slower)
			regressions = append(regressions, JobRegression{
				Name:            name,
				URLs:            urls,
				OldAvgDuration:  oldAvg,
				NewAvgDuration:  newAvg,
				PercentIncrease: percentChange,
				AbsoluteChange:  change,
				Changepoint:     cp,
			})
		} else {
			// Improvement (got faster)
			improvements = append(improvements, JobImprovement{
				Name:            name,
				URLs:            urls,
				OldAvgDuration:  oldAvg,
				NewAvgDuration:  newAvg,
				PercentDecrease: -percentChange,
				AbsoluteChange:  -change,
				Changepoint:     cp,
			})
		}
	}

	// Sort by percent change (worst first)
	sort.Slice(regressions, func(i, j int) bool {
		return regressions[i].PercentIncrease > regressions[j].PercentIncrease
	})
	sort.Slice(improvements, func(i, j int) bool {
		return improvements[i].PercentDecrease > improvements[j].PercentDecrease
	})

	// Limit to top 10
	if len(regressions) > 10 {
		regressions = regressions[:10]
	}
	if len(improvements) > 10 {
		improvements = improvements[:10]
	}

	return regressions, improvements
}

// calculateQueueTimeStats computes queue time statistics
func calculateQueueTimeStats(runs []RunData) QueueTimeStats {
	var queueTimes []float64
	var runTimes []float64

	for _, run := range runs {
		for _, job := range run.Jobs {
			if job.QueueTime > 0 {
				queueTimes = append(queueTimes, float64(job.QueueTime)/1000.0)
			}
			if job.Duration > 0 {
				runTimes = append(runTimes, float64(job.Duration)/1000.0)
			}
		}
	}

	if len(queueTimes) == 0 {
		return QueueTimeStats{}
	}

	avgQueue := average(queueTimes)
	avgRun := average(runTimes)
	totalTime := avgQueue + avgRun
	queueRatio := 0.0
	if totalTime > 0 {
		queueRatio = (avgQueue / totalTime) * 100
	}

	return QueueTimeStats{
		AvgQueueTime:    avgQueue,
		MedianQueueTime: calculateMedian(queueTimes),
		P95QueueTime:    calculatePercentile(queueTimes, 95),
		AvgRunTime:      avgRun,
		MedianRunTime:   calculateMedian(runTimes),
		QueueTimeRatio:  queueRatio,
	}
}
