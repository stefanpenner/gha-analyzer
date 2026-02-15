package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
)

type PendingJobWithSource struct {
	analyzer.PendingJob
	SourceURL  string
	SourceName string
}

type CommitAggregate struct {
	Name                    string
	URLIndex                int
	TotalRunsForCommit      int
	TotalComputeMsForCommit int64
}

func sortByEarliest(results []analyzer.URLResult) []analyzer.URLResult {
	sorted := append([]analyzer.URLResult{}, results...)
	sortURLResults(sorted)
	return sorted
}

func boolYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n", title)
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", len(title)))
}
