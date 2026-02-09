package analyzer

import (
	"fmt"
	"time"

	"github.com/stefanpenner/gha-analyzer/pkg/storage"
)

// SaveURLResult converts and saves a URLResult to storage
func SaveURLResult(store storage.Storage, result URLResult) error {
	// Convert URLResult to WorkflowRun
	run := &storage.WorkflowRun{
		Owner:       result.Owner,
		Repo:        result.Repo,
		RunID:       0, // We don't have a single run ID for combined results
		HeadSHA:     result.HeadSHA,
		Branch:      result.BranchName,
		Event:       "",
		Status:      "completed",
		Conclusion:  determineConclusion(result.Metrics.SuccessRate),
		StartTime:   time.UnixMilli(result.EarliestTime),
		EndTime:     calculateEndTime(result.Metrics.JobTimeline),
		Duration:    calculateDuration(result.Metrics.JobTimeline),
		SuccessRate: parseSuccessRate(result.Metrics.SuccessRate),
		TotalJobs:   result.Metrics.TotalJobs,
		FailedJobs:  result.Metrics.FailedJobs,
	}

	if err := store.SaveRun(run); err != nil {
		return err
	}

	// Save individual jobs
	for _, job := range result.Metrics.JobTimeline {
		j := &storage.Job{
			RunID:      run.ID,
			JobID:      0, // Generate unique ID or use URL hash
			Name:       job.Name,
			Status:     job.Status,
			Conclusion: job.Conclusion,
			StartTime:  time.UnixMilli(job.StartTime),
			EndTime:    time.UnixMilli(job.EndTime),
			Duration:   job.EndTime - job.StartTime,
			RunnerType: "",
		}

		if err := store.SaveJob(j); err != nil {
			return err
		}
	}

	return nil
}

func determineConclusion(successRate string) string {
	rate := parseSuccessRate(successRate)
	if rate >= 100.0 {
		return "success"
	} else if rate > 0 {
		return "failure"
	}
	return "cancelled"
}

func parseSuccessRate(rateStr string) float64 {
	var rate float64
	_, _ = fmt.Sscanf(rateStr, "%f", &rate)
	return rate
}

func calculateEndTime(jobs []TimelineJob) time.Time {
	if len(jobs) == 0 {
		return time.Now()
	}

	latest := jobs[0].EndTime
	for _, job := range jobs {
		if job.EndTime > latest {
			latest = job.EndTime
		}
	}

	return time.UnixMilli(latest)
}

func calculateDuration(jobs []TimelineJob) int64 {
	if len(jobs) == 0 {
		return 0
	}

	earliest := jobs[0].StartTime
	latest := jobs[0].EndTime

	for _, job := range jobs {
		if job.StartTime < earliest {
			earliest = job.StartTime
		}
		if job.EndTime > latest {
			latest = job.EndTime
		}
	}

	return latest - earliest
}
