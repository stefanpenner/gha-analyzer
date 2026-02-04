package polling

import (
	"context"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/githubapi"
)

type PollingIngestor struct {
	client   *githubapi.Client
	urls     []string
	reporter analyzer.ProgressReporter
	opts     analyzer.AnalyzeOptions
}

func NewPollingIngestor(client *githubapi.Client, urls []string, reporter analyzer.ProgressReporter, opts analyzer.AnalyzeOptions) *PollingIngestor {
	return &PollingIngestor{
		client:   client,
		urls:     urls,
		reporter: reporter,
		opts:     opts,
	}
}

func (i *PollingIngestor) Ingest(ctx context.Context) ([]analyzer.URLResult, int64, int64, error) {
	results, _, globalEarliest, globalLatest, errs := analyzer.AnalyzeURLs(ctx, i.urls, i.client, i.reporter, i.opts)
	if len(errs) > 0 {
		// Return the full URLError which includes the URL context
		return nil, 0, 0, errs[0]
	}

	return results, globalEarliest, globalLatest, nil
}
