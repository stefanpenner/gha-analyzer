package output

import (
	"sort"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
)

func sortReviewEvents(events []analyzer.ReviewEvent) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].TimeMillis() < events[j].TimeMillis()
	})
}

func sortURLResults(results []analyzer.URLResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].EarliestTime < results[j].EarliestTime
	})
}

func sortTimelineJobs(jobs []analyzer.TimelineJob, less func(a, b analyzer.TimelineJob) bool) {
	sort.Slice(jobs, func(i, j int) bool {
		return less(jobs[i], jobs[j])
	})
}

func sortGroupNames(groupNames []string, less func(a, b string) bool) {
	sort.Slice(groupNames, func(i, j int) bool {
		return less(groupNames[i], groupNames[j])
	})
}

func requiredEmoji(isRequired bool) string {
	if isRequired {
		return " 🔒"
	}
	return ""
}
