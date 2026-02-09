package analyzer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
)

// TrendAnalysis contains trend data for a repository
type TrendAnalysis struct {
	Owner            string
	Repo             string
	TimeRange        TimeRange
	Summary          TrendSummary
	DurationTrend    []DataPoint
	SuccessRateTrend []DataPoint
	JobTrends        []JobTrend
	FlakyJobs        []FlakyJob
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
	ID         int64
	Name       string
	Status     string
	Conclusion string
	StartedAt  time.Time
	CompletedAt time.Time
	Duration   int64 // milliseconds
}

// AnalyzeTrends analyzes historical trends for a repository using GitHub API
func AnalyzeTrends(ctx context.Context, client githubapi.GitHubProvider, owner, repo string, days int, branch, workflow string) (*TrendAnalysis, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(days) * 24 * time.Hour)

	// Fetch workflow runs from GitHub
	runs, err := client.FetchRecentWorkflowRuns(ctx, owner, repo, days, branch, workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no workflow runs found for %s/%s in the last %d days", owner, repo, days)
	}

	// Convert to RunData and fetch jobs
	runData, err := convertAndFetchJobs(ctx, client, runs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch job data: %w", err)
	}

	analysis := &TrendAnalysis{
		Owner: owner,
		Repo:  repo,
		TimeRange: TimeRange{
			Start: startTime,
			End:   endTime,
			Days:  days,
		},
	}

	// Calculate summary statistics
	analysis.Summary = calculateTrendSummary(runData)

	// Generate duration trend
	analysis.DurationTrend = generateDurationTrend(runData, days)

	// Generate success rate trend
	analysis.SuccessRateTrend = generateSuccessRateTrend(runData, days)

	// Analyze individual jobs
	analysis.JobTrends = analyzeJobTrends(runData)

	// Detect flaky jobs
	analysis.FlakyJobs = detectFlakyJobs(runData)
	analysis.Summary.MostFlakyJobsCount = len(analysis.FlakyJobs)

	return analysis, nil
}

// convertAndFetchJobs converts workflow runs and fetches job details
func convertAndFetchJobs(ctx context.Context, client githubapi.GitHubProvider, runs []githubapi.WorkflowRun) ([]RunData, error) {
	runData := make([]RunData, 0, len(runs))

	for _, run := range runs {
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

		// Fetch jobs for this run
		jobsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/jobs",
			run.Repository.Owner.Login, run.Repository.Name, run.ID)
		jobs, err := client.FetchJobsPaginated(ctx, jobsURL)
		if err == nil {
			for _, job := range jobs {
				startedAt, _ := utils.ParseTime(job.StartedAt)
				completedAt, _ := utils.ParseTime(job.CompletedAt)

				duration := int64(0)
				if !startedAt.IsZero() && !completedAt.IsZero() {
					duration = completedAt.Sub(startedAt).Milliseconds()
				}

				rd.Jobs = append(rd.Jobs, JobData{
					ID:          job.ID,
					Name:        job.Name,
					Status:      job.Status,
					Conclusion:  job.Conclusion,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Duration:    duration,
				})
			}
		}

		// Calculate run duration
		if !createdAt.IsZero() && !updatedAt.IsZero() {
			rd.Duration = updatedAt.Sub(createdAt).Milliseconds()
		}

		runData = append(runData, rd)
	}

	return runData, nil
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

	return TrendSummary{
		TotalRuns:      len(runs),
		AvgDuration:    avgDuration,
		MedianDuration: medianDuration,
		P95Duration:    p95Duration,
		AvgSuccessRate: avgSuccessRate,
		TrendDirection: trendDirection,
		PercentChange:  percentChange,
	}
}

// generateDurationTrend creates time-series data for duration
func generateDurationTrend(runs []RunData, days int) []DataPoint {
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
func generateSuccessRateTrend(runs []RunData, days int) []DataPoint {
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

		trends = append(trends, JobTrend{
			Name:           name,
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
