package analyzer

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/storage"
)

// TrendAnalysis contains trend data for a repository
type TrendAnalysis struct {
	Owner           string
	Repo            string
	TimeRange       TimeRange
	Summary         TrendSummary
	DurationTrend   []DataPoint
	SuccessRateTrend []DataPoint
	JobTrends       []JobTrend
	FlakyJobs       []FlakyJob
}

// TimeRange represents a time period
type TimeRange struct {
	Start time.Time
	End   time.Time
	Days  int
}

// TrendSummary provides high-level statistics
type TrendSummary struct {
	TotalRuns            int
	AvgDuration          float64
	MedianDuration       float64
	P95Duration          float64
	AvgSuccessRate       float64
	TrendDirection       string // "improving", "stable", "degrading"
	PercentChange        float64
	MostFlakyJobsCount   int
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

// AnalyzeTrends analyzes historical trends for a repository
func AnalyzeTrends(store storage.Storage, owner, repo string, days int) (*TrendAnalysis, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(days) * 24 * time.Hour)

	// Get all runs in the time range
	runs, err := store.GetRuns(owner, repo, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no historical data found for %s/%s", owner, repo)
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
	analysis.Summary = calculateSummary(runs)

	// Generate duration trend
	analysis.DurationTrend = generateDurationTrend(runs, days)

	// Generate success rate trend
	analysis.SuccessRateTrend = generateSuccessRateTrend(runs, days)

	// Analyze individual jobs
	jobTrends, flakyJobs, err := analyzeJobs(store, owner, repo, startTime, runs)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze jobs: %w", err)
	}
	analysis.JobTrends = jobTrends
	analysis.FlakyJobs = flakyJobs
	analysis.Summary.MostFlakyJobsCount = len(flakyJobs)

	return analysis, nil
}

// calculateSummary computes summary statistics
func calculateSummary(runs []*storage.WorkflowRun) TrendSummary {
	if len(runs) == 0 {
		return TrendSummary{}
	}

	durations := make([]float64, 0, len(runs))
	totalDuration := 0.0
	successCount := 0

	for _, run := range runs {
		if run.Duration > 0 {
			durationSec := float64(run.Duration) / 1000.0
			durations = append(durations, durationSec)
			totalDuration += durationSec
		}
		if run.Conclusion == "success" {
			successCount++
		}
	}

	avgDuration := 0.0
	if len(durations) > 0 {
		avgDuration = totalDuration / float64(len(durations))
	}

	medianDuration := calculateMedian(durations)
	p95Duration := calculatePercentile(durations, 95)
	avgSuccessRate := float64(successCount) / float64(len(runs)) * 100

	// Determine trend direction by comparing first half to second half
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
func generateDurationTrend(runs []*storage.WorkflowRun, days int) []DataPoint {
	// Group runs by day
	dayBuckets := make(map[string][]float64)

	for _, run := range runs {
		if run.Duration > 0 {
			dayKey := run.StartTime.Format("2006-01-02")
			durationSec := float64(run.Duration) / 1000.0
			dayBuckets[dayKey] = append(dayBuckets[dayKey], durationSec)
		}
	}

	// Create data points
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

	// Sort by timestamp
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points
}

// generateSuccessRateTrend creates time-series data for success rate
func generateSuccessRateTrend(runs []*storage.WorkflowRun, days int) []DataPoint {
	// Group runs by day
	dayBuckets := make(map[string]struct{ success, total int })

	for _, run := range runs {
		dayKey := run.StartTime.Format("2006-01-02")
		bucket := dayBuckets[dayKey]
		bucket.total++
		if run.Conclusion == "success" {
			bucket.success++
		}
		dayBuckets[dayKey] = bucket
	}

	// Create data points
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

	// Sort by timestamp
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points
}

// analyzeJobs analyzes individual job trends and identifies flaky jobs
func analyzeJobs(store storage.Storage, owner, repo string, since time.Time, runs []*storage.WorkflowRun) ([]JobTrend, []FlakyJob, error) {
	// Collect all unique job names from recent runs
	jobNames := make(map[string]bool)
	for _, run := range runs {
		jobs, err := store.GetJobs(run.ID)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			jobNames[job.Name] = true
		}
	}

	var jobTrends []JobTrend
	var flakyJobs []FlakyJob

	// Analyze each job
	for jobName := range jobNames {
		history, err := store.GetJobHistory(owner, repo, jobName, since)
		if err != nil || len(history) == 0 {
			continue
		}

		// Calculate job trend
		trend := analyzeJobTrend(jobName, history)
		jobTrends = append(jobTrends, trend)

		// Check if job is flaky
		flaky := detectFlakyJob(jobName, history)
		if flaky != nil && flaky.FlakeRate > 10 { // Only include if >10% flake rate
			flakyJobs = append(flakyJobs, *flaky)
		}
	}

	// Sort by average duration (slowest first)
	sort.Slice(jobTrends, func(i, j int) bool {
		return jobTrends[i].AvgDuration > jobTrends[j].AvgDuration
	})

	// Sort flaky jobs by flake rate (worst first)
	sort.Slice(flakyJobs, func(i, j int) bool {
		return flakyJobs[i].FlakeRate > flakyJobs[j].FlakeRate
	})

	return jobTrends, flakyJobs, nil
}

// analyzeJobTrend analyzes trend for a single job
func analyzeJobTrend(name string, history []*storage.Job) JobTrend {
	durations := make([]float64, 0, len(history))
	successCount := 0

	for _, job := range history {
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
	successRate := float64(successCount) / float64(len(history)) * 100

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

	// Create duration points (daily averages)
	points := groupJobsByDay(history)

	return JobTrend{
		Name:           name,
		AvgDuration:    avgDuration,
		MedianDuration: medianDuration,
		SuccessRate:    successRate,
		TotalRuns:      len(history),
		TrendDirection: trendDirection,
		DurationPoints: points,
	}
}

// detectFlakyJob identifies if a job exhibits flaky behavior
func detectFlakyJob(name string, history []*storage.Job) *FlakyJob {
	if len(history) < 5 {
		return nil // Not enough data
	}

	successCount := 0
	failureCount := 0
	var lastFailure time.Time

	// Count recent failures (last 10 runs)
	recentFailures := 0
	recentLimit := 10
	if len(history) < recentLimit {
		recentLimit = len(history)
	}

	for i, job := range history {
		if job.Conclusion == "success" {
			successCount++
		} else if job.Conclusion == "failure" {
			failureCount++
			if lastFailure.IsZero() || job.EndTime.After(lastFailure) {
				lastFailure = job.EndTime
			}

			if i < recentLimit {
				recentFailures++
			}
		}
	}

	if failureCount == 0 {
		return nil // Never failed, not flaky
	}

	flakeRate := float64(failureCount) / float64(len(history)) * 100

	return &FlakyJob{
		Name:           name,
		TotalRuns:      len(history),
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		FlakeRate:      flakeRate,
		RecentFailures: recentFailures,
		LastFailure:    lastFailure,
	}
}

// groupJobsByDay groups jobs by day and calculates daily averages
func groupJobsByDay(jobs []*storage.Job) []DataPoint {
	dayBuckets := make(map[string][]float64)

	for _, job := range jobs {
		if job.Duration > 0 {
			dayKey := job.StartTime.Format("2006-01-02")
			durationSec := float64(job.Duration) / 1000.0
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
