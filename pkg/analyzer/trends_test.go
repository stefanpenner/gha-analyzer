package analyzer

import (
	"fmt"
	"testing"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
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

// Tests covering the trend ordering bug fix.
//
// The bug: calculateTrendSummary, analyzeJobTrends, and calculateJobChanges
// split the runs slice at the midpoint and compare "first half" vs "second half"
// to determine trend direction. They assume chronological order (oldest first),
// so the first half represents older data and the second half represents newer data.
//
// The GitHub API returns runs in newest-first order. If data was passed through
// unsorted, the halves would be reversed, causing improving trends to be
// reported as degrading and vice versa.
//
// The fix sorts runs chronologically in AnalyzeTrends before passing them to
// these functions. These tests verify the functions produce correct results
// when given properly chronologically-ordered data.

// TestCalculateTrendSummary_ChronologicalOrder verifies that calculateTrendSummary
// correctly identifies improving, degrading, and stable trends when runs are
// provided in chronological order (oldest first), as the fix ensures.
func TestCalculateTrendSummary_ChronologicalOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("older runs slow newer runs fast is improving", func(t *testing.T) {
		t.Parallel()
		// Chronological order: oldest (slow) -> newest (fast)
		// First half avg = (120+110)/2 = 115s, second half avg = (50+40)/2 = 45s
		// Percent change = (45-115)/115 * 100 = -60.9% (< -5%, so improving)
		runs := []RunData{
			makeRunData("success", 120000, now.Add(-8*time.Hour), nil), // oldest, 120s
			makeRunData("success", 110000, now.Add(-6*time.Hour), nil), // 110s
			makeRunData("success", 50000, now.Add(-4*time.Hour), nil),  // 50s
			makeRunData("success", 40000, now.Add(-2*time.Hour), nil),  // newest, 40s
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "improving", summary.TrendDirection,
			"runs getting faster over time should be 'improving'")
		assert.Less(t, summary.PercentChange, -5.0,
			"percent change should be significantly negative for improving trend")
	})

	t.Run("older runs fast newer runs slow is degrading", func(t *testing.T) {
		t.Parallel()
		// Chronological order: oldest (fast) -> newest (slow)
		// First half avg = (40+50)/2 = 45s, second half avg = (110+120)/2 = 115s
		// Percent change = (115-45)/45 * 100 = +155.6% (> 5%, so degrading)
		runs := []RunData{
			makeRunData("success", 40000, now.Add(-8*time.Hour), nil),  // oldest, 40s
			makeRunData("success", 50000, now.Add(-6*time.Hour), nil),  // 50s
			makeRunData("success", 110000, now.Add(-4*time.Hour), nil), // 110s
			makeRunData("success", 120000, now.Add(-2*time.Hour), nil), // newest, 120s
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "degrading", summary.TrendDirection,
			"runs getting slower over time should be 'degrading'")
		assert.Greater(t, summary.PercentChange, 5.0,
			"percent change should be significantly positive for degrading trend")
	})

	t.Run("similar durations across time is stable", func(t *testing.T) {
		t.Parallel()
		// Chronological order with similar durations throughout
		// First half avg = (60+62)/2 = 61s, second half avg = (58+60)/2 = 59s
		// Percent change = (59-61)/61 * 100 = -3.3% (between -5% and 5%, so stable)
		runs := []RunData{
			makeRunData("success", 60000, now.Add(-8*time.Hour), nil), // oldest, 60s
			makeRunData("success", 62000, now.Add(-6*time.Hour), nil), // 62s
			makeRunData("success", 58000, now.Add(-4*time.Hour), nil), // 58s
			makeRunData("success", 60000, now.Add(-2*time.Hour), nil), // newest, 60s
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "stable", summary.TrendDirection,
			"runs with similar durations across time should be 'stable'")
	})

	t.Run("six runs with gradual improvement", func(t *testing.T) {
		t.Parallel()
		// More data points to ensure the midpoint split works with larger sets.
		// First half (indices 0-2): 180s, 170s, 160s -> avg 170s
		// Second half (indices 3-5): 80s, 70s, 60s -> avg 70s
		// Percent change = (70-170)/170 * 100 = -58.8% (improving)
		runs := []RunData{
			makeRunData("success", 180000, now.Add(-12*time.Hour), nil), // oldest
			makeRunData("success", 170000, now.Add(-10*time.Hour), nil),
			makeRunData("success", 160000, now.Add(-8*time.Hour), nil),
			makeRunData("success", 80000, now.Add(-6*time.Hour), nil),
			makeRunData("success", 70000, now.Add(-4*time.Hour), nil),
			makeRunData("success", 60000, now.Add(-2*time.Hour), nil), // newest
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "improving", summary.TrendDirection)
		assert.Equal(t, 6, summary.TotalRuns)
	})

	t.Run("six runs with gradual degradation", func(t *testing.T) {
		t.Parallel()
		// First half (indices 0-2): 60s, 70s, 80s -> avg 70s
		// Second half (indices 3-5): 160s, 170s, 180s -> avg 170s
		// Percent change = (170-70)/70 * 100 = +142.9% (degrading)
		runs := []RunData{
			makeRunData("success", 60000, now.Add(-12*time.Hour), nil), // oldest
			makeRunData("success", 70000, now.Add(-10*time.Hour), nil),
			makeRunData("success", 80000, now.Add(-8*time.Hour), nil),
			makeRunData("success", 160000, now.Add(-6*time.Hour), nil),
			makeRunData("success", 170000, now.Add(-4*time.Hour), nil),
			makeRunData("success", 180000, now.Add(-2*time.Hour), nil), // newest
		}
		summary := calculateTrendSummary(runs)
		assert.Equal(t, "degrading", summary.TrendDirection)
		assert.Equal(t, 6, summary.TotalRuns)
	})

	t.Run("trend description populated for improving", func(t *testing.T) {
		t.Parallel()
		runs := []RunData{
			makeRunData("success", 120000, now.Add(-8*time.Hour), nil),
			makeRunData("success", 110000, now.Add(-6*time.Hour), nil),
			makeRunData("success", 50000, now.Add(-4*time.Hour), nil),
			makeRunData("success", 40000, now.Add(-2*time.Hour), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.NotEmpty(t, summary.TrendDescription)
		assert.Contains(t, summary.TrendDescription, "decreased")
	})

	t.Run("trend description populated for degrading", func(t *testing.T) {
		t.Parallel()
		runs := []RunData{
			makeRunData("success", 40000, now.Add(-8*time.Hour), nil),
			makeRunData("success", 50000, now.Add(-6*time.Hour), nil),
			makeRunData("success", 110000, now.Add(-4*time.Hour), nil),
			makeRunData("success", 120000, now.Add(-2*time.Hour), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.NotEmpty(t, summary.TrendDescription)
		assert.Contains(t, summary.TrendDescription, "increased")
	})

	t.Run("trend description populated for stable", func(t *testing.T) {
		t.Parallel()
		runs := []RunData{
			makeRunData("success", 60000, now.Add(-8*time.Hour), nil),
			makeRunData("success", 62000, now.Add(-6*time.Hour), nil),
			makeRunData("success", 58000, now.Add(-4*time.Hour), nil),
			makeRunData("success", 60000, now.Add(-2*time.Hour), nil),
		}
		summary := calculateTrendSummary(runs)
		assert.NotEmpty(t, summary.TrendDescription)
		assert.Contains(t, summary.TrendDescription, "stable")
	})
}

// TestCalculateJobChanges_ChronologicalOrder verifies that calculateJobChanges
// correctly identifies regressions and improvements when runs are provided in
// chronological order. The first half of the slice represents older runs and
// the second half represents newer runs.
func TestCalculateJobChanges_ChronologicalOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("jobs getting slower over time are regressions", func(t *testing.T) {
		t.Parallel()
		// Chronological order: older runs have short job durations,
		// newer runs have long job durations.
		// First half (older): "build" at 10s, 12s -> avg 11s
		// Second half (newer): "build" at 25s, 30s -> avg 27.5s
		// Change = (27.5-11)/11 * 100 = +150% (regression, > 10% threshold)
		runs := []RunData{
			makeRunData("success", 15000, now.Add(-8*time.Hour), []JobData{
				{Name: "build", Duration: 10000, Conclusion: "success"},
			}),
			makeRunData("success", 17000, now.Add(-6*time.Hour), []JobData{
				{Name: "build", Duration: 12000, Conclusion: "success"},
			}),
			makeRunData("success", 30000, now.Add(-4*time.Hour), []JobData{
				{Name: "build", Duration: 25000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-2*time.Hour), []JobData{
				{Name: "build", Duration: 30000, Conclusion: "success"},
			}),
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Len(t, regressions, 1, "should detect one regression")
		assert.Equal(t, "build", regressions[0].Name)
		assert.Greater(t, regressions[0].PercentIncrease, 10.0,
			"percent increase should exceed the 10% significance threshold")
		assert.Greater(t, regressions[0].NewAvgDuration, regressions[0].OldAvgDuration,
			"new average should be higher than old average for a regression")
		assert.Empty(t, improvements, "no improvements expected when jobs got slower")
	})

	t.Run("jobs getting faster over time are improvements", func(t *testing.T) {
		t.Parallel()
		// Chronological order: older runs have long job durations,
		// newer runs have short job durations.
		// First half (older): "test" at 60s, 55s -> avg 57.5s
		// Second half (newer): "test" at 20s, 15s -> avg 17.5s
		// Change = (17.5-57.5)/57.5 * 100 = -69.6% (improvement)
		runs := []RunData{
			makeRunData("success", 65000, now.Add(-8*time.Hour), []JobData{
				{Name: "test", Duration: 60000, Conclusion: "success"},
			}),
			makeRunData("success", 60000, now.Add(-6*time.Hour), []JobData{
				{Name: "test", Duration: 55000, Conclusion: "success"},
			}),
			makeRunData("success", 25000, now.Add(-4*time.Hour), []JobData{
				{Name: "test", Duration: 20000, Conclusion: "success"},
			}),
			makeRunData("success", 20000, now.Add(-2*time.Hour), []JobData{
				{Name: "test", Duration: 15000, Conclusion: "success"},
			}),
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Empty(t, regressions, "no regressions expected when jobs got faster")
		assert.Len(t, improvements, 1, "should detect one improvement")
		assert.Equal(t, "test", improvements[0].Name)
		assert.Greater(t, improvements[0].PercentDecrease, 10.0,
			"percent decrease should exceed the 10% significance threshold")
		assert.Less(t, improvements[0].NewAvgDuration, improvements[0].OldAvgDuration,
			"new average should be lower than old average for an improvement")
	})

	t.Run("multiple jobs with mixed changes", func(t *testing.T) {
		t.Parallel()
		// Chronological order with two jobs: "build" regresses, "lint" improves.
		// build: first half 10s,12s (avg 11s) -> second half 25s,28s (avg 26.5s) = regression
		// lint: first half 30s,28s (avg 29s) -> second half 10s,8s (avg 9s) = improvement
		runs := []RunData{
			makeRunData("success", 35000, now.Add(-8*time.Hour), []JobData{
				{Name: "build", Duration: 10000, Conclusion: "success"},
				{Name: "lint", Duration: 30000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-6*time.Hour), []JobData{
				{Name: "build", Duration: 12000, Conclusion: "success"},
				{Name: "lint", Duration: 28000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-4*time.Hour), []JobData{
				{Name: "build", Duration: 25000, Conclusion: "success"},
				{Name: "lint", Duration: 10000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-2*time.Hour), []JobData{
				{Name: "build", Duration: 28000, Conclusion: "success"},
				{Name: "lint", Duration: 8000, Conclusion: "success"},
			}),
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Len(t, regressions, 1, "should detect build as a regression")
		assert.Equal(t, "build", regressions[0].Name)
		assert.Len(t, improvements, 1, "should detect lint as an improvement")
		assert.Equal(t, "lint", improvements[0].Name)
	})

	t.Run("six runs regression detection", func(t *testing.T) {
		t.Parallel()
		// Larger dataset with 6 runs. The midpoint is at index 3.
		// First half (indices 0-2): "deploy" at 5s, 6s, 5s -> avg 5.33s
		// Second half (indices 3-5): "deploy" at 15s, 16s, 17s -> avg 16s
		// Change = (16-5.33)/5.33 * 100 = +200% (regression)
		runs := []RunData{
			makeRunData("success", 10000, now.Add(-12*time.Hour), []JobData{
				{Name: "deploy", Duration: 5000, Conclusion: "success"},
			}),
			makeRunData("success", 10000, now.Add(-10*time.Hour), []JobData{
				{Name: "deploy", Duration: 6000, Conclusion: "success"},
			}),
			makeRunData("success", 10000, now.Add(-8*time.Hour), []JobData{
				{Name: "deploy", Duration: 5000, Conclusion: "success"},
			}),
			makeRunData("success", 20000, now.Add(-6*time.Hour), []JobData{
				{Name: "deploy", Duration: 15000, Conclusion: "success"},
			}),
			makeRunData("success", 20000, now.Add(-4*time.Hour), []JobData{
				{Name: "deploy", Duration: 16000, Conclusion: "success"},
			}),
			makeRunData("success", 22000, now.Add(-2*time.Hour), []JobData{
				{Name: "deploy", Duration: 17000, Conclusion: "success"},
			}),
		}
		regressions, improvements := calculateJobChanges(runs)
		assert.Len(t, regressions, 1)
		assert.Equal(t, "deploy", regressions[0].Name)
		assert.Greater(t, regressions[0].PercentIncrease, 100.0,
			"a tripling of duration should show >100% increase")
		assert.Empty(t, improvements)
	})
}

// TestAnalyzeJobTrends_ChronologicalOrder verifies that analyzeJobTrends
// correctly determines trend direction for individual jobs when runs are
// provided in chronological order (oldest first).
//
// analyzeJobTrends collects all JobData instances for each job name across
// all runs (preserving the order they appear in the runs slice), then splits
// the collected durations at the midpoint to compare first half vs second half.
func TestAnalyzeJobTrends_ChronologicalOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("job duration increasing over time is degrading", func(t *testing.T) {
		t.Parallel()
		// Chronological order: "ci" job gets progressively slower.
		// Job durations in order: 10s, 12s, 30s, 35s
		// First half avg = (10+12)/2 = 11s, second half avg = (30+35)/2 = 32.5s
		// Percent change = (32.5-11)/11 * 100 = +195.5% (degrading)
		runs := []RunData{
			makeRunData("success", 15000, now.Add(-8*time.Hour), []JobData{
				{Name: "ci", Duration: 10000, Conclusion: "success"},
			}),
			makeRunData("success", 17000, now.Add(-6*time.Hour), []JobData{
				{Name: "ci", Duration: 12000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-4*time.Hour), []JobData{
				{Name: "ci", Duration: 30000, Conclusion: "success"},
			}),
			makeRunData("success", 40000, now.Add(-2*time.Hour), []JobData{
				{Name: "ci", Duration: 35000, Conclusion: "success"},
			}),
		}
		trends := analyzeJobTrends(runs)
		assert.Len(t, trends, 1)
		assert.Equal(t, "ci", trends[0].Name)
		assert.Equal(t, "degrading", trends[0].TrendDirection,
			"job getting slower over time should be 'degrading'")
		assert.Equal(t, 4, trends[0].TotalRuns)
	})

	t.Run("job duration decreasing over time is improving", func(t *testing.T) {
		t.Parallel()
		// Chronological order: "ci" job gets progressively faster.
		// Job durations in order: 60s, 55s, 20s, 15s
		// First half avg = (60+55)/2 = 57.5s, second half avg = (20+15)/2 = 17.5s
		// Percent change = (17.5-57.5)/57.5 * 100 = -69.6% (improving)
		runs := []RunData{
			makeRunData("success", 65000, now.Add(-8*time.Hour), []JobData{
				{Name: "ci", Duration: 60000, Conclusion: "success"},
			}),
			makeRunData("success", 60000, now.Add(-6*time.Hour), []JobData{
				{Name: "ci", Duration: 55000, Conclusion: "success"},
			}),
			makeRunData("success", 25000, now.Add(-4*time.Hour), []JobData{
				{Name: "ci", Duration: 20000, Conclusion: "success"},
			}),
			makeRunData("success", 20000, now.Add(-2*time.Hour), []JobData{
				{Name: "ci", Duration: 15000, Conclusion: "success"},
			}),
		}
		trends := analyzeJobTrends(runs)
		assert.Len(t, trends, 1)
		assert.Equal(t, "ci", trends[0].Name)
		assert.Equal(t, "improving", trends[0].TrendDirection,
			"job getting faster over time should be 'improving'")
	})

	t.Run("job with stable duration is stable", func(t *testing.T) {
		t.Parallel()
		// Chronological order with roughly constant job durations.
		// Job durations: 30s, 31s, 29s, 30s
		// First half avg = (30+31)/2 = 30.5s, second half avg = (29+30)/2 = 29.5s
		// Percent change = (29.5-30.5)/30.5 * 100 = -3.3% (stable, within +/-5%)
		runs := []RunData{
			makeRunData("success", 35000, now.Add(-8*time.Hour), []JobData{
				{Name: "ci", Duration: 30000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-6*time.Hour), []JobData{
				{Name: "ci", Duration: 31000, Conclusion: "success"},
			}),
			makeRunData("success", 34000, now.Add(-4*time.Hour), []JobData{
				{Name: "ci", Duration: 29000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-2*time.Hour), []JobData{
				{Name: "ci", Duration: 30000, Conclusion: "success"},
			}),
		}
		trends := analyzeJobTrends(runs)
		assert.Len(t, trends, 1)
		assert.Equal(t, "ci", trends[0].Name)
		assert.Equal(t, "stable", trends[0].TrendDirection,
			"job with roughly constant duration should be 'stable'")
	})

	t.Run("multiple jobs with different trends", func(t *testing.T) {
		t.Parallel()
		// Two jobs: "build" degrades, "lint" improves over time.
		// build durations in order: 10s, 11s, 25s, 28s (degrading)
		// lint durations in order: 40s, 38s, 15s, 12s (improving)
		runs := []RunData{
			makeRunData("success", 50000, now.Add(-8*time.Hour), []JobData{
				{Name: "build", Duration: 10000, Conclusion: "success"},
				{Name: "lint", Duration: 40000, Conclusion: "success"},
			}),
			makeRunData("success", 50000, now.Add(-6*time.Hour), []JobData{
				{Name: "build", Duration: 11000, Conclusion: "success"},
				{Name: "lint", Duration: 38000, Conclusion: "success"},
			}),
			makeRunData("success", 40000, now.Add(-4*time.Hour), []JobData{
				{Name: "build", Duration: 25000, Conclusion: "success"},
				{Name: "lint", Duration: 15000, Conclusion: "success"},
			}),
			makeRunData("success", 40000, now.Add(-2*time.Hour), []JobData{
				{Name: "build", Duration: 28000, Conclusion: "success"},
				{Name: "lint", Duration: 12000, Conclusion: "success"},
			}),
		}
		trends := analyzeJobTrends(runs)
		assert.Len(t, trends, 2)

		// Find each job's trend (sorted by avg duration descending)
		trendMap := make(map[string]string)
		for _, jt := range trends {
			trendMap[jt.Name] = jt.TrendDirection
		}
		assert.Equal(t, "degrading", trendMap["build"],
			"build job getting slower should be 'degrading'")
		assert.Equal(t, "improving", trendMap["lint"],
			"lint job getting faster should be 'improving'")
	})

	t.Run("six runs with gradual job degradation", func(t *testing.T) {
		t.Parallel()
		// 6 runs, midpoint at index 3.
		// "test" durations: 10s, 11s, 12s | 30s, 32s, 35s
		// First half avg = 11s, second half avg = 32.3s (degrading)
		runs := []RunData{
			makeRunData("success", 15000, now.Add(-12*time.Hour), []JobData{
				{Name: "test", Duration: 10000, Conclusion: "success"},
			}),
			makeRunData("success", 15000, now.Add(-10*time.Hour), []JobData{
				{Name: "test", Duration: 11000, Conclusion: "success"},
			}),
			makeRunData("success", 15000, now.Add(-8*time.Hour), []JobData{
				{Name: "test", Duration: 12000, Conclusion: "success"},
			}),
			makeRunData("success", 35000, now.Add(-6*time.Hour), []JobData{
				{Name: "test", Duration: 30000, Conclusion: "success"},
			}),
			makeRunData("success", 37000, now.Add(-4*time.Hour), []JobData{
				{Name: "test", Duration: 32000, Conclusion: "success"},
			}),
			makeRunData("success", 40000, now.Add(-2*time.Hour), []JobData{
				{Name: "test", Duration: 35000, Conclusion: "success"},
			}),
		}
		trends := analyzeJobTrends(runs)
		assert.Len(t, trends, 1)
		assert.Equal(t, "test", trends[0].Name)
		assert.Equal(t, "degrading", trends[0].TrendDirection)
		assert.Equal(t, 6, trends[0].TotalRuns)
		assert.Equal(t, 100.0, trends[0].SuccessRate)
	})
}

func TestCalculateSampleSize(t *testing.T) {
	t.Parallel()

	t.Run("zero runs", func(t *testing.T) {
		assert.Equal(t, 0, calculateSampleSize(0, 0.95, 0.10))
	})

	t.Run("small population returns all", func(t *testing.T) {
		// For very small N, the sample size should equal N
		size := calculateSampleSize(5, 0.95, 0.10)
		assert.Equal(t, 5, size)
	})

	t.Run("known values at 95% confidence 10% margin", func(t *testing.T) {
		// n₀ = 1.96² × 0.25 / 0.01 = 96.04
		// n = 96.04 / (1 + 95.04/N)

		// N=50: n = 96.04 / (1 + 95.04/50) = 96.04 / 2.9008 ≈ 33.1 -> 34
		assert.Equal(t, 34, calculateSampleSize(50, 0.95, 0.10))

		// N=100: n = 96.04 / (1 + 95.04/100) = 96.04 / 1.9504 ≈ 49.2 -> 50
		assert.Equal(t, 50, calculateSampleSize(100, 0.95, 0.10))

		// N=300: n = 96.04 / (1 + 95.04/300) = 96.04 / 1.3168 ≈ 72.9 -> 73
		assert.Equal(t, 73, calculateSampleSize(300, 0.95, 0.10))

		// N=1000: n = 96.04 / (1 + 95.04/1000) = 96.04 / 1.09504 ≈ 87.7 -> 88
		assert.Equal(t, 88, calculateSampleSize(1000, 0.95, 0.10))
	})

	t.Run("sample never exceeds population", func(t *testing.T) {
		for _, n := range []int{1, 2, 10, 50, 100, 1000} {
			size := calculateSampleSize(n, 0.95, 0.10)
			assert.LessOrEqual(t, size, n)
			assert.Greater(t, size, 0)
		}
	})
}

func TestSampleRunIndices(t *testing.T) {
	t.Parallel()

	makeRuns := func(n int) []githubapi.WorkflowRun {
		runs := make([]githubapi.WorkflowRun, n)
		for i := range runs {
			runs[i] = githubapi.WorkflowRun{ID: int64(1000 + i)}
		}
		return runs
	}

	t.Run("sample size equals total returns all indices", func(t *testing.T) {
		runs := makeRuns(5)
		indices := sampleRunIndices(runs, 5)
		assert.Equal(t, []int{0, 1, 2, 3, 4}, indices)
	})

	t.Run("sample size greater than total returns all indices", func(t *testing.T) {
		runs := makeRuns(3)
		indices := sampleRunIndices(runs, 10)
		assert.Equal(t, []int{0, 1, 2}, indices)
	})

	t.Run("correct count returned", func(t *testing.T) {
		runs := makeRuns(100)
		indices := sampleRunIndices(runs, 20)
		assert.Len(t, indices, 20)
	})

	t.Run("no duplicate indices", func(t *testing.T) {
		runs := makeRuns(100)
		indices := sampleRunIndices(runs, 50)
		seen := make(map[int]bool)
		for _, idx := range indices {
			assert.False(t, seen[idx], "duplicate index: %d", idx)
			seen[idx] = true
		}
	})

	t.Run("all indices are valid", func(t *testing.T) {
		runs := makeRuns(100)
		indices := sampleRunIndices(runs, 30)
		for _, idx := range indices {
			assert.GreaterOrEqual(t, idx, 0)
			assert.Less(t, idx, 100)
		}
	})

	t.Run("indices are sorted", func(t *testing.T) {
		runs := makeRuns(100)
		indices := sampleRunIndices(runs, 40)
		for i := 1; i < len(indices); i++ {
			assert.Less(t, indices[i-1], indices[i])
		}
	})

	t.Run("deterministic for same input", func(t *testing.T) {
		runs := makeRuns(100)
		indices1 := sampleRunIndices(runs, 30)
		indices2 := sampleRunIndices(runs, 30)
		assert.Equal(t, indices1, indices2)
	})

	t.Run("different runs produce different samples", func(t *testing.T) {
		runs1 := makeRuns(100)
		runs2 := make([]githubapi.WorkflowRun, 100)
		for i := range runs2 {
			runs2[i] = githubapi.WorkflowRun{ID: int64(9000 + i)}
		}
		indices1 := sampleRunIndices(runs1, 30)
		indices2 := sampleRunIndices(runs2, 30)
		assert.NotEqual(t, indices1, indices2)
	})
}

func TestDetectChangepoint(t *testing.T) {
	t.Parallel()

	t.Run("too few observations returns nil", func(t *testing.T) {
		t.Parallel()
		obs := []jobObservation{
			{DurationSec: 10}, {DurationSec: 10}, {DurationSec: 20}, {DurationSec: 20},
		}
		// minSideSize=3 requires at least 6 observations
		cp := detectChangepoint(obs, 3)
		assert.Nil(t, cp)
	})

	t.Run("clear step function finds changepoint at boundary", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		obs := make([]jobObservation, 10)
		for i := range obs {
			dur := 10.0
			if i >= 5 {
				dur = 20.0
			}
			obs[i] = jobObservation{
				DurationSec:  dur,
				RunCreatedAt: now.Add(time.Duration(i) * time.Hour),
				HeadSHA:      fmt.Sprintf("sha%d", i),
				JobURL:       fmt.Sprintf("https://example.com/job/%d", i),
			}
		}
		cp := detectChangepoint(obs, 3)
		assert.NotNil(t, cp)
		assert.Equal(t, 5, cp.Index)
		assert.Equal(t, 10, cp.TotalPoints)
		assert.Equal(t, 10.0, cp.BeforeAvg)
		assert.Equal(t, 20.0, cp.AfterAvg)
	})

	t.Run("asymmetric changepoint not at midpoint", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		// [10,10,10,20,20,20,20,20,20,20] — shift at index 3
		obs := make([]jobObservation, 10)
		for i := range obs {
			dur := 20.0
			if i < 3 {
				dur = 10.0
			}
			obs[i] = jobObservation{
				DurationSec:  dur,
				RunCreatedAt: now.Add(time.Duration(i) * time.Hour),
				HeadSHA:      fmt.Sprintf("sha%d", i),
				JobURL:       fmt.Sprintf("https://example.com/job/%d", i),
			}
		}
		cp := detectChangepoint(obs, 3)
		assert.NotNil(t, cp)
		assert.Equal(t, 3, cp.Index)
	})

	t.Run("preserves metadata from boundary observations", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		obs := make([]jobObservation, 8)
		for i := range obs {
			dur := 10.0
			if i >= 4 {
				dur = 20.0
			}
			obs[i] = jobObservation{
				DurationSec:  dur,
				RunCreatedAt: now.Add(time.Duration(i) * time.Hour),
				HeadSHA:      fmt.Sprintf("sha%d", i),
				JobURL:       fmt.Sprintf("https://example.com/job/%d", i),
			}
		}
		cp := detectChangepoint(obs, 3)
		assert.NotNil(t, cp)
		assert.Equal(t, "sha3", cp.BeforeSHA)  // last before changepoint
		assert.Equal(t, "sha4", cp.AfterSHA)   // first after changepoint
		assert.Equal(t, obs[4].RunCreatedAt, cp.Date)
		assert.Equal(t, "https://example.com/job/3", cp.BeforeRunURL)
		assert.Equal(t, "https://example.com/job/4", cp.AfterRunURL)
	})
}

func TestCalculateJobChanges_Changepoint(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("changepoint populated on regression with enough data", func(t *testing.T) {
		t.Parallel()
		// 8 runs: first 4 have build=10s, last 4 have build=20s
		runs := make([]RunData, 8)
		for i := range runs {
			dur := int64(10000)
			if i >= 4 {
				dur = 20000
			}
			runs[i] = RunData{
				HeadSHA:   fmt.Sprintf("sha%d", i),
				CreatedAt: now.Add(time.Duration(i) * time.Hour),
				Jobs: []JobData{{
					Name:     "build",
					Duration: dur,
					URL:      fmt.Sprintf("https://example.com/job/%d", i),
				}},
			}
		}
		regressions, _ := calculateJobChanges(runs)
		assert.Len(t, regressions, 1)
		assert.NotNil(t, regressions[0].Changepoint, "changepoint should be populated with 8 observations")
		assert.Equal(t, 4, regressions[0].Changepoint.Index)
	})

	t.Run("changepoint populated on improvement with enough data", func(t *testing.T) {
		t.Parallel()
		runs := make([]RunData, 8)
		for i := range runs {
			dur := int64(20000)
			if i >= 4 {
				dur = 10000
			}
			runs[i] = RunData{
				HeadSHA:   fmt.Sprintf("sha%d", i),
				CreatedAt: now.Add(time.Duration(i) * time.Hour),
				Jobs: []JobData{{
					Name:     "build",
					Duration: dur,
					URL:      fmt.Sprintf("https://example.com/job/%d", i),
				}},
			}
		}
		_, improvements := calculateJobChanges(runs)
		assert.Len(t, improvements, 1)
		assert.NotNil(t, improvements[0].Changepoint, "changepoint should be populated with 8 observations")
		assert.Equal(t, 4, improvements[0].Changepoint.Index)
	})

	t.Run("changepoint nil when fewer than 6 observations", func(t *testing.T) {
		t.Parallel()
		// 4 runs: regression detected but too few for changepoint (minSideSize=3 needs 6)
		runs := []RunData{
			{CreatedAt: now.Add(-4 * time.Hour), Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{CreatedAt: now.Add(-3 * time.Hour), Jobs: []JobData{{Name: "build", Duration: 10000}}},
			{CreatedAt: now.Add(-2 * time.Hour), Jobs: []JobData{{Name: "build", Duration: 25000}}},
			{CreatedAt: now.Add(-1 * time.Hour), Jobs: []JobData{{Name: "build", Duration: 25000}}},
		}
		regressions, _ := calculateJobChanges(runs)
		assert.Len(t, regressions, 1)
		assert.Nil(t, regressions[0].Changepoint, "changepoint should be nil with only 4 observations")
	})
}

func TestSelectSamplePages(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for single page", func(t *testing.T) {
		pages := selectSamplePages(1, 1, 42)
		assert.Nil(t, pages)
	})

	t.Run("returns nil for pagesNeeded <= 1", func(t *testing.T) {
		pages := selectSamplePages(10, 1, 42)
		assert.Nil(t, pages)
	})

	t.Run("returns all remaining pages when needed >= available", func(t *testing.T) {
		pages := selectSamplePages(5, 10, 42)
		assert.Equal(t, []int{2, 3, 4, 5}, pages) // pages 2-5 (page 1 excluded)
	})

	t.Run("correct count returned", func(t *testing.T) {
		pages := selectSamplePages(50, 10, 42)
		assert.Len(t, pages, 9) // pagesNeeded-1 = 9 (page 1 already fetched)
	})

	t.Run("pages are in valid range", func(t *testing.T) {
		pages := selectSamplePages(100, 10, 42)
		for _, p := range pages {
			assert.GreaterOrEqual(t, p, 2, "page should be >= 2")
			assert.LessOrEqual(t, p, 100, "page should be <= totalPages")
		}
	})

	t.Run("pages are sorted", func(t *testing.T) {
		pages := selectSamplePages(100, 20, 42)
		for i := 1; i < len(pages); i++ {
			assert.LessOrEqual(t, pages[i-1], pages[i])
		}
	})

	t.Run("no duplicate pages", func(t *testing.T) {
		pages := selectSamplePages(224, 10, 42)
		seen := make(map[int]bool)
		for _, p := range pages {
			assert.False(t, seen[p], "duplicate page: %d", p)
			seen[p] = true
		}
	})

	t.Run("deterministic for same seed", func(t *testing.T) {
		pages1 := selectSamplePages(100, 10, 42)
		pages2 := selectSamplePages(100, 10, 42)
		assert.Equal(t, pages1, pages2)
	})

	t.Run("different seeds produce different selections", func(t *testing.T) {
		pages1 := selectSamplePages(100, 10, 42)
		pages2 := selectSamplePages(100, 10, 999)
		assert.NotEqual(t, pages1, pages2)
	})

	t.Run("stratified distribution covers range", func(t *testing.T) {
		// With 224 pages and 10 needed, pages should be spread across the range
		pages := selectSamplePages(224, 10, 42)
		assert.Len(t, pages, 9)
		// First selected page should be from the early range
		assert.Less(t, pages[0], 50)
		// Last selected page should be from the late range
		assert.Greater(t, pages[len(pages)-1], 150)
	})
}

func TestGenerateRationale(t *testing.T) {
	t.Parallel()

	t.Run("run and job sampling", func(t *testing.T) {
		s := SamplingInfo{
			Enabled:        true,
			SampleSize:     94,
			TotalRuns:       500,
			TotalAvailable: 22354,
			RunSampled:     true,
			PagesFetched:   5,
			TotalPages:     224,
			Confidence:     0.95,
			MarginOfError:  0.10,
		}
		r := generateRationale(s)
		assert.Contains(t, r, "22,354")
		assert.Contains(t, r, "500 fetched")
		assert.Contains(t, r, "5 of 224 pages")
		assert.Contains(t, r, "94 sampled for job details")
		assert.Contains(t, r, "95% confidence")
		assert.Contains(t, r, "±10% margin")
	})

	t.Run("job-only sampling", func(t *testing.T) {
		s := SamplingInfo{
			Enabled:        true,
			SampleSize:     73,
			TotalRuns:       150,
			TotalAvailable: 150,
			RunSampled:     false,
			Confidence:     0.95,
			MarginOfError:  0.10,
		}
		r := generateRationale(s)
		assert.Contains(t, r, "150 runs analyzed")
		assert.Contains(t, r, "73 sampled for job details")
		assert.NotContains(t, r, "pages")
	})

	t.Run("no sampling", func(t *testing.T) {
		s := SamplingInfo{
			Enabled:        false,
			SampleSize:     42,
			TotalRuns:       42,
			TotalAvailable: 42,
			RunSampled:     false,
			Confidence:     0.95,
			MarginOfError:  0.10,
		}
		r := generateRationale(s)
		assert.Contains(t, r, "42 runs analyzed")
		assert.Contains(t, r, "Full job details")
	})
}

func TestFormatCount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0", formatCount(0))
	assert.Equal(t, "42", formatCount(42))
	assert.Equal(t, "999", formatCount(999))
	assert.Equal(t, "1,000", formatCount(1000))
	assert.Equal(t, "22,354", formatCount(22354))
	assert.Equal(t, "1,000,000", formatCount(1000000))
}

func TestConvertRuns(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		result := convertRuns(nil)
		assert.Empty(t, result)
	})

	t.Run("converts fields correctly", func(t *testing.T) {
		runs := []githubapi.WorkflowRun{
			{
				ID:         42,
				HeadSHA:    "abc123",
				Status:     "completed",
				Conclusion: "success",
				CreatedAt:  "2026-01-15T10:00:00Z",
				UpdatedAt:  "2026-01-15T10:05:00Z",
			},
		}
		result := convertRuns(runs)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(42), result[0].ID)
		assert.Equal(t, "abc123", result[0].HeadSHA)
		assert.Equal(t, "success", result[0].Conclusion)
		assert.Equal(t, int64(300000), result[0].Duration) // 5 minutes in ms
		assert.Empty(t, result[0].Jobs)
	})

	t.Run("preserves order", func(t *testing.T) {
		runs := []githubapi.WorkflowRun{
			{ID: 1, CreatedAt: "2026-01-15T10:00:00Z", UpdatedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, CreatedAt: "2026-01-15T11:00:00Z", UpdatedAt: "2026-01-15T11:00:00Z"},
			{ID: 3, CreatedAt: "2026-01-15T12:00:00Z", UpdatedAt: "2026-01-15T12:00:00Z"},
		}
		result := convertRuns(runs)
		assert.Len(t, result, 3)
		assert.Equal(t, int64(1), result[0].ID)
		assert.Equal(t, int64(2), result[1].ID)
		assert.Equal(t, int64(3), result[2].ID)
	})
}
