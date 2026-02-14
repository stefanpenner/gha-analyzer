package analyzer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makeRunData(conclusion string, durationMs int64, createdAt time.Time, jobs []JobData) RunData {
	return RunData{
		ID:         1,
		Status:     "completed",
		Conclusion: conclusion,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt.Add(time.Duration(durationMs) * time.Millisecond),
		Duration:   durationMs,
		Jobs:       jobs,
	}
}

func TestCalculateTrendSummary(t *testing.T) {
	t.Parallel()

	t.Run("empty runs", func(t *testing.T) {
		summary := calculateTrendSummary(nil)
		assert.Equal(t, 0, summary.TotalRuns)
		assert.Equal(t, 0.0, summary.AvgDuration)
	})

	t.Run("single run", func(t *testing.T) {
		runs := []RunData{
			makeRunData("success", 60000, time.Now(), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, 1, summary.TotalRuns)
		assert.Equal(t, 60.0, summary.AvgDuration)
		assert.Equal(t, 100.0, summary.AvgSuccessRate)
		assert.Equal(t, "stable", summary.TrendDirection)
	})

	t.Run("mixed success and failure", func(t *testing.T) {
		runs := []RunData{
			makeRunData("success", 60000, time.Now(), nil),
			makeRunData("failure", 30000, time.Now(), nil),
			makeRunData("success", 90000, time.Now(), nil),
			makeRunData("failure", 45000, time.Now(), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, 4, summary.TotalRuns)
		assert.Equal(t, 50.0, summary.AvgSuccessRate)
	})

	t.Run("improving trend", func(t *testing.T) {
		// First half slow, second half fast
		runs := []RunData{
			makeRunData("success", 100000, time.Now(), nil),
			makeRunData("success", 100000, time.Now(), nil),
			makeRunData("success", 50000, time.Now(), nil),
			makeRunData("success", 50000, time.Now(), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "improving", summary.TrendDirection)
		assert.Less(t, summary.PercentChange, -5.0)
	})

	t.Run("degrading trend", func(t *testing.T) {
		// First half fast, second half slow
		runs := []RunData{
			makeRunData("success", 50000, time.Now(), nil),
			makeRunData("success", 50000, time.Now(), nil),
			makeRunData("success", 100000, time.Now(), nil),
			makeRunData("success", 100000, time.Now(), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "degrading", summary.TrendDirection)
		assert.Greater(t, summary.PercentChange, 5.0)
	})
}

func TestGenerateDurationTrend(t *testing.T) {
	t.Parallel()

	t.Run("empty runs", func(t *testing.T) {
		points := generateDurationTrend(nil)
		assert.Empty(t, points)
	})

	t.Run("groups by day", func(t *testing.T) {
		day1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
		runs := []RunData{
			makeRunData("success", 60000, day1, nil),
			makeRunData("success", 120000, day1, nil),
			makeRunData("success", 180000, day2, nil),
		}
		points := generateDurationTrend(runs)
		assert.Len(t, points, 2)
		// Points should be sorted by time
		assert.True(t, points[0].Timestamp.Before(points[1].Timestamp))
	})
}

func TestGenerateSuccessRateTrend(t *testing.T) {
	t.Parallel()

	t.Run("empty runs", func(t *testing.T) {
		points := generateSuccessRateTrend(nil)
		assert.Empty(t, points)
	})

	t.Run("calculates daily success rate", func(t *testing.T) {
		day1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
		runs := []RunData{
			makeRunData("success", 60000, day1, nil),
			makeRunData("failure", 60000, day1, nil),
		}
		points := generateSuccessRateTrend(runs)
		assert.Len(t, points, 1)
		assert.Equal(t, 50.0, points[0].Value)
		assert.Equal(t, 2, points[0].Count)
	})
}

func TestDetectFlakyJobs(t *testing.T) {
	t.Parallel()

	t.Run("no runs", func(t *testing.T) {
		flakyJobs := detectFlakyJobs(nil)
		assert.Empty(t, flakyJobs)
	})

	t.Run("too few runs ignored", func(t *testing.T) {
		now := time.Now()
		runs := []RunData{
			{Jobs: []JobData{{Name: "test", Conclusion: "failure", CompletedAt: now}}},
			{Jobs: []JobData{{Name: "test", Conclusion: "failure", CompletedAt: now}}},
		}
		flakyJobs := detectFlakyJobs(runs)
		assert.Empty(t, flakyJobs)
	})

	t.Run("detects flaky job above threshold", func(t *testing.T) {
		now := time.Now()
		runs := make([]RunData, 10)
		for i := range runs {
			conclusion := "success"
			if i < 3 { // 30% failure rate
				conclusion = "failure"
			}
			runs[i] = RunData{Jobs: []JobData{{
				Name:        "test",
				Conclusion:  conclusion,
				CompletedAt: now.Add(time.Duration(i) * time.Hour),
			}}}
		}
		flakyJobs := detectFlakyJobs(runs)
		assert.Len(t, flakyJobs, 1)
		assert.Equal(t, "test", flakyJobs[0].Name)
		assert.Equal(t, 30.0, flakyJobs[0].FlakeRate)
	})

	t.Run("ignores stable jobs below threshold", func(t *testing.T) {
		now := time.Now()
		runs := make([]RunData, 20)
		for i := range runs {
			conclusion := "success"
			if i == 0 { // 5% failure rate
				conclusion = "failure"
			}
			runs[i] = RunData{Jobs: []JobData{{
				Name:        "test",
				Conclusion:  conclusion,
				CompletedAt: now.Add(time.Duration(i) * time.Hour),
			}}}
		}
		flakyJobs := detectFlakyJobs(runs)
		assert.Empty(t, flakyJobs)
	})
}

func TestCalculateJobChanges(t *testing.T) {
	t.Parallel()

	t.Run("too few runs", func(t *testing.T) {
		regressions, improvements := calculateJobChanges([]RunData{{}, {}})
		assert.Nil(t, regressions)
		assert.Nil(t, improvements)
	})

	t.Run("detects regression", func(t *testing.T) {
		runs := []RunData{
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{Jobs: []JobData{{Name: "build", Duration: 20000}}},
			{Jobs: []JobData{{Name: "build", Duration: 20000}}},
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Len(t, regressions, 1)
		assert.Equal(t, "build", regressions[0].Name)
		assert.Greater(t, regressions[0].PercentIncrease, 10.0)
		assert.Empty(t, improvements)
	})

	t.Run("detects improvement", func(t *testing.T) {
		runs := []RunData{
			{Jobs: []JobData{{Name: "build", Duration: 20000}}},
			{Jobs: []JobData{{Name: "build", Duration: 20000}}},
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Empty(t, regressions)
		assert.Len(t, improvements, 1)
		assert.Equal(t, "build", improvements[0].Name)
		assert.Greater(t, improvements[0].PercentDecrease, 10.0)
	})

	t.Run("ignores small changes", func(t *testing.T) {
		runs := []RunData{
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{Jobs: []JobData{{Name: "build", Duration: 10500}}},
			{Jobs: []JobData{{Name: "build", Duration: 10500}}},
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Empty(t, regressions)
		assert.Empty(t, improvements)
	})
}

func TestCalculateQueueTimeStats(t *testing.T) {
	t.Parallel()

	t.Run("empty runs", func(t *testing.T) {
		stats := calculateQueueTimeStats(nil)
		assert.Equal(t, 0.0, stats.AvgQueueTime)
	})

	t.Run("calculates queue stats", func(t *testing.T) {
		runs := []RunData{
			{Jobs: []JobData{
				{QueueTime: 5000, Duration: 60000},
				{QueueTime: 3000, Duration: 30000},
			}},
		}
		stats := calculateQueueTimeStats(runs)
		assert.Equal(t, 4.0, stats.AvgQueueTime) // (5+3)/2 seconds
		assert.Equal(t, 45.0, stats.AvgRunTime)   // (60+30)/2 seconds
		assert.Greater(t, stats.QueueTimeRatio, 0.0)
		assert.Less(t, stats.QueueTimeRatio, 100.0)
	})
}

func TestStatisticalFunctions(t *testing.T) {
	t.Parallel()

	t.Run("average", func(t *testing.T) {
		assert.Equal(t, 0.0, average(nil))
		assert.Equal(t, 2.0, average([]float64{1, 2, 3}))
	})

	t.Run("calculateMedian", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateMedian(nil))
		assert.Equal(t, 2.0, calculateMedian([]float64{1, 2, 3}))
		assert.Equal(t, 2.5, calculateMedian([]float64{1, 2, 3, 4}))
	})

	t.Run("calculatePercentile", func(t *testing.T) {
		assert.Equal(t, 0.0, calculatePercentile(nil, 95))
		values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		p95 := calculatePercentile(values, 95)
		assert.Equal(t, 10.0, p95)
	})
}
