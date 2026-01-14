package analyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateMaxConcurrency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		starts []JobEvent
		ends   []JobEvent
		expect int
	}{
		{
			name:   "overlapping",
			starts: []JobEvent{{Ts: 1000, Type: "start"}, {Ts: 2000, Type: "start"}, {Ts: 3000, Type: "start"}},
			ends:   []JobEvent{{Ts: 4000, Type: "end"}, {Ts: 3500, Type: "end"}, {Ts: 5000, Type: "end"}},
			expect: 3,
		},
		{
			name:   "empty",
			starts: nil,
			ends:   nil,
			expect: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, CalculateMaxConcurrency(tc.starts, tc.ends))
		})
	}
}

func TestCalculateFinalMetrics(t *testing.T) {
	t.Parallel()

	metrics := Metrics{
		TotalRuns:      10,
		SuccessfulRuns: 8,
		FailedRuns:     2,
		TotalJobs:      20,
		FailedJobs:     3,
		TotalSteps:     100,
		FailedSteps:    5,
		JobDurations:   []float64{1000, 2000, 3000, 4000, 5000},
		StepDurations: []StepDuration{
			{Duration: 100},
			{Duration: 200},
			{Duration: 300},
		},
	}

	final := CalculateFinalMetrics(metrics, 10, nil, nil)
	assert.Equal(t, 3000.0, final.AvgJobDuration)
	assert.Equal(t, 200.0, final.AvgStepDuration)
	assert.Equal(t, "80.0", final.SuccessRate)
	assert.Equal(t, "85.0", final.JobSuccessRate)
}

func TestFindBottleneckJobs(t *testing.T) {
	t.Parallel()

	jobs := []TimelineJob{
		{Name: "FastJob", StartTime: 1000, EndTime: 2000},
		{Name: "SlowJob", StartTime: 1500, EndTime: 10000},
		{Name: "MediumJob", StartTime: 2000, EndTime: 5000},
	}
	bottlenecks := FindBottleneckJobs(jobs)
	assert.NotEmpty(t, bottlenecks)
	assert.Equal(t, "SlowJob", bottlenecks[0].Name)
}

func TestAnalyzeSlowJobsAndSteps(t *testing.T) {
	t.Parallel()

	metrics := Metrics{
		JobDurations: []float64{1000, 5000, 2000, 8000, 3000},
		JobNames:     []string{"Job1", "Job2", "Job3", "Job4", "Job5"},
		JobURLs:      []string{"url1", "url2", "url3", "url4", "url5"},
		StepDurations: []StepDuration{
			{Name: "Step1", Duration: 1000},
			{Name: "Step2", Duration: 5000},
			{Name: "Step3", Duration: 2000},
			{Name: "Step4", Duration: 8000},
		},
	}
	slowJobs := AnalyzeSlowJobs(metrics, 3)
	assert.Len(t, slowJobs, 3)
	assert.Equal(t, "Job4", slowJobs[0].Name)

	slowSteps := AnalyzeSlowSteps(metrics, 2)
	assert.Len(t, slowSteps, 2)
	assert.Equal(t, "Step4", slowSteps[0].Name)
}

func TestFindOverlappingJobs(t *testing.T) {
	t.Parallel()

	jobs := []TimelineJob{
		{Name: "Job1", StartTime: 1000, EndTime: 3000},
		{Name: "Job2", StartTime: 2000, EndTime: 4000},
		{Name: "Job3", StartTime: 5000, EndTime: 7000},
	}
	overlaps := FindOverlappingJobs(jobs)
	assert.Len(t, overlaps, 1)
	assert.Equal(t, "Job1", overlaps[0][0].Name)
	assert.Equal(t, "Job2", overlaps[0][1].Name)
}
